// Copyright 2019 The Fuchsia Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package dhcp

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"net"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"

	bufferv2 "github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/buffer"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/checksum"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/udp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/waiter"
)

const (
	tag                        = "DHCP"
	defaultLeaseLength Seconds = 12 * 3600

	// stateRecentHistoryLength is the length of recent DHCP state transitions.
	// A value large enough must be used to allow the last 24h to be recorded even
	// when the DHCP lease is renewed every hour. And if the DHCP state machine
	// breaks and cycles through state very fast, this will be enough data to
	// possibly find a pattern.
	stateRecentHistoryLength = 128

	// As per RFC 2131 section 3.1,
	//
	//   If the client detects that the address is already in use (e.g., through
	//   the use of ARP), the client MUST send a DHCPDECLINE message to the server
	//   and restarts the configuration process. The client SHOULD wait a minimum
	//   of ten seconds before restarting the configuration process to avoid
	//   excessive network traffic in case of looping.
	minBackoffAfterDupAddrDetetected = 10 * time.Second
)

// Based on RFC 2131 Sec. 4.4.5, this defaults to (0.5 * duration_of_lease).
func defaultRenewTime(leaseLength Seconds) Seconds { return leaseLength / 2 }

// Based on RFC 2131 Sec. 4.4.5, this defaults to (0.875 * duration_of_lease).
func defaultRebindTime(leaseLength Seconds) Seconds { return (leaseLength * 875) / 1000 }

type AcquiredFunc func(ctx context.Context, lost, acquired tcpip.AddressWithPrefix, cfg Config)

// Client is a DHCP client.
type Client struct {
	overrideLinkAddr tcpip.LinkAddress
	stack            *stack.Stack
	networkEndpoint  stack.NetworkEndpoint
	xid              xid

	// info holds the Client's state as type Info.
	info atomic.Value

	stats Stats

	acquiredFunc AcquiredFunc

	wq waiter.Queue

	// Used to ensure that only one Run goroutine per interface may be
	// permitted to run at a time. In certain cases, rapidly flapping the
	// DHCP client on and off can cause a second instance of Run to start
	// before the existing one has finished, which can violate invariants.
	// At the time of writing, TestDhcpConfiguration was creating this
	// scenario and causing panics.
	sem chan struct{}

	// Stubbable in test.
	rand               *rand.Rand
	retransTimeout     func(time.Duration) <-chan time.Time
	acquire            func(context.Context, *Client, string, *Info) (Config, error)
	now                func() time.Time
	contextWithTimeout func(context.Context, time.Duration) (context.Context, context.CancelFunc)
}

type PacketDiscardStats struct {
	InvalidPort       *tcpip.IntegralStatCounterMap
	InvalidTransProto *tcpip.IntegralStatCounterMap
	InvalidPacketType *tcpip.IntegralStatCounterMap
}

func (p *PacketDiscardStats) Init() {
	p.InvalidPort = &tcpip.IntegralStatCounterMap{}
	p.InvalidTransProto = &tcpip.IntegralStatCounterMap{}
	p.InvalidPacketType = &tcpip.IntegralStatCounterMap{}

	p.InvalidPort.Init()
	p.InvalidTransProto.Init()
	p.InvalidPacketType.Init()
}

// Stats collects DHCP statistics per client.
type Stats struct {
	PacketDiscardStats          PacketDiscardStats
	InitAcquire                 tcpip.StatCounter
	RenewAcquire                tcpip.StatCounter
	RebindAcquire               tcpip.StatCounter
	SendDiscovers               tcpip.StatCounter
	RecvOffers                  tcpip.StatCounter
	SendRequests                tcpip.StatCounter
	RecvAcks                    tcpip.StatCounter
	RecvNaks                    tcpip.StatCounter
	SendDiscoverErrors          tcpip.StatCounter
	SendRequestErrors           tcpip.StatCounter
	RecvOfferErrors             tcpip.StatCounter
	RecvOfferUnexpectedType     tcpip.StatCounter
	RecvOfferOptsDecodeErrors   tcpip.StatCounter
	RecvOfferTimeout            tcpip.StatCounter
	RecvOfferAcquisitionTimeout tcpip.StatCounter
	RecvOfferNoServerAddress    tcpip.StatCounter
	RecvAckErrors               tcpip.StatCounter
	RecvNakErrors               tcpip.StatCounter
	RecvAckOptsDecodeErrors     tcpip.StatCounter
	RecvAckAddrErrors           tcpip.StatCounter
	RecvAckUnexpectedType       tcpip.StatCounter
	RecvAckTimeout              tcpip.StatCounter
	RecvAckAcquisitionTimeout   tcpip.StatCounter
	ReacquireAfterNAK           tcpip.StatCounter
}

type Info struct {
	// NICID is the identifer to the associated NIC.
	NICID tcpip.NICID
	// LinkAddr is the link-address of the associated NIC.
	LinkAddr tcpip.LinkAddress
	// Acquisition is the duration within which a complete DHCP transaction must
	// complete before timing out.
	Acquisition time.Duration
	// Backoff is the duration for which the client must wait before starting a
	// new DHCP transaction after a failed transaction.
	Backoff time.Duration
	// Retransmission is the duration to wait before resending a DISCOVER or
	// REQUEST within an active transaction.
	Retransmission time.Duration
	// Acquired is the network address most recently acquired by the client.
	Acquired tcpip.AddressWithPrefix
	// State is the DHCP client state.
	State dhcpClientState
	// Assigned is the network address added by the client to its stack.
	Assigned tcpip.AddressWithPrefix
	// LeaseExpiration is the time at which the client's current lease will
	// expire.
	LeaseExpiration time.Time
	// RenewTime is the at which the client will transition to its renewing state.
	RenewTime time.Time
	// RebindTime is the time at which the client will transition to its rebinding
	// state.
	RebindTime time.Time
	// Config is the last DHCP configuration assigned to the client by the server.
	Config Config
}

// NewClient creates a DHCP client.
//
// acquiredFunc will be called after each DHCP acquisition, and is responsible
// for making necessary modifications to the stack state.
func NewClient(
	s *stack.Stack,
	nicid tcpip.NICID,
	acquisition,
	backoff,
	retransmission time.Duration,
	acquiredFunc AcquiredFunc,
) *Client {
	ep, err := s.GetNetworkEndpoint(nicid, header.IPv4ProtocolNumber)
	if err != nil {
		panic(fmt.Sprintf("stack.GetNetworkEndpoint(%d, header.IPv4ProtocolNumber): %s", nicid, err))
	}
	c := &Client{
		stack:           s,
		networkEndpoint: ep,
		acquiredFunc:    acquiredFunc,
		sem:             make(chan struct{}, 1),
		rand:            rand.New(rand.NewSource(time.Now().UnixNano())),
		retransTimeout:  time.After,
		acquire:         acquire,
		now:             time.Now,
		contextWithTimeout: func(ctx context.Context, duration time.Duration) (context.Context, context.CancelFunc) {
			return context.WithTimeout(ctx, duration)
		},
	}
	c.stats.PacketDiscardStats.Init()
	c.storeInfo(&Info{
		NICID:          nicid,
		Acquisition:    acquisition,
		Retransmission: retransmission,
		Backoff:        backoff,
	})
	return c
}

func (c *Client) SetOverrideLinkAddr(addr tcpip.LinkAddress) {
	c.overrideLinkAddr = addr
}

// Info returns a copy of the synchronized state of the Info.
func (c *Client) Info() Info {
	return c.info.Load().(Info)
}

// storeInfo updates the synchronized copy of the DHCP Info and if the client's
// state changed, it will log it in the state recent history.
//
// Because of the size of Info, it is passed as a pointer to avoid an extra
// unnecessary copy.
func (c *Client) storeInfo(info *Info) {
	c.info.Store(*info)
}

// Stats returns a reference to the Client`s stats.
func (c *Client) Stats() *Stats {
	return &c.stats
}

// Run runs the DHCP client, returning the address assigned when it is stopped.
//
// The function periodically searches for a new IP address.
func (c *Client) Run(ctx context.Context) (rtn tcpip.AddressWithPrefix) {
	info := c.Info()
	//log.Infof(tag+" Starting DHCP client with info: %v", info)

	nicName := c.stack.FindNICNameFromID(info.NICID)
	log.Infof(tag+" Found NIC name: %s for ID: %d", nicName, info.NICID)

	// For the initial iteration of the acquisition loop, the client should
	// be in the initSelecting state, corresponding to the
	// INIT->SELECTING->REQUESTING->BOUND state transition:
	// https://tools.ietf.org/html/rfc2131#section-4.4
	info.State = initSelecting
	log.Infof(tag+" %s: Setting initial state to: %s", nicName, info.State)

	c.sem <- struct{}{}
	log.Infof(tag+" %s: Acquired semaphore", nicName)
	defer func() { <-c.sem }()
	defer func() {
		log.Infof(tag+" %s: client is stopping, cleaning up", nicName)
		rtn = c.cleanup(&info, nicName, true /* release */)
	}()

	for {
		if err := func() error {
			acquisitionTimeout := info.Acquisition
			txnStart := c.now()

			log.Infof(tag+" %s: Starting acquisition with timeout %v", nicName, acquisitionTimeout)

			// Adjust the timeout to make sure client is not stuck in retransmission
			// when it should transition to the next state. This can only happen for
			// two time-driven transitions: RENEW->REBIND, REBIND->INIT.
			//
			// Another time-driven transition BOUND->RENEW is not handled here because
			// the client does not have to send out any request during BOUND.
			switch s := info.State; s {
			case initSelecting:
				// Nothing to do. The client is initializing, no leases have been acquired.
				// Thus no times are set for renew, rebind, and lease expiration.
				c.stats.InitAcquire.Increment()
				log.Infof(tag+" %s: In initSelecting state", nicName)
			case renewing:
				c.stats.RenewAcquire.Increment()
				log.Infof(tag+" %s: In renewing state", nicName)
				if tilRebind := info.RebindTime.Sub(txnStart); tilRebind < acquisitionTimeout {
					acquisitionTimeout = tilRebind
					log.Infof(tag+" %s: Adjusted acquisition timeout to %v for rebind", nicName, acquisitionTimeout)
				}
			case rebinding:
				c.stats.RebindAcquire.Increment()
				log.Infof(tag+" %s: In rebinding state", nicName)
				if tilLeaseExpire := info.LeaseExpiration.Sub(txnStart); tilLeaseExpire < acquisitionTimeout {
					acquisitionTimeout = tilLeaseExpire
					log.Infof(tag+" %s: Adjusted acquisition timeout to %v for lease expiration", nicName, acquisitionTimeout)
				}
			default:
				panic(fmt.Sprintf("unexpected state before acquire: %s", s))
			}

			ctxAcquire, cancel := c.contextWithTimeout(ctx, acquisitionTimeout)
			defer cancel()

			log.Infof(tag+" %s: Attempting to acquire configuration", nicName)
			cfg, err := c.acquire(ctxAcquire, c, nicName, &info)
			if err != nil {
				log.Infof(tag+" %s: Failed to acquire configuration: %v", nicName, err)
				return err
			}
			if cfg.Declined {
				c.stats.ReacquireAfterNAK.Increment()
				log.Infof(tag+" %s: Configuration declined, cleaning up", nicName)
				c.lost(ctx, c.cleanup(&info, nicName, false /* release */))
				return nil
			}

			if cfg.LeaseLength == 0 {
				log.Infof(tag+" %s: Unspecified lease length, using default %v", nicName, defaultLeaseLength)
				cfg.LeaseLength = defaultLeaseLength
			}
			{
				renewTime := defaultRenewTime(cfg.LeaseLength)
				if cfg.RenewTime == 0 {
					log.Infof(tag+" %s: Unspecified renew time, using default %v", nicName, renewTime)
					cfg.RenewTime = renewTime
				}
				if cfg.RenewTime >= cfg.LeaseLength {
					log.Infof(tag+" %s: Renew time %v >= lease length %v, using default %v", nicName, cfg.RenewTime, cfg.LeaseLength, renewTime)
					cfg.RenewTime = renewTime
				}
			}
			{
				rebindTime := defaultRebindTime(cfg.LeaseLength)
				if cfg.RebindTime == 0 {
					log.Infof(tag+" %s: Unspecified rebind time, using default %v", nicName, rebindTime)
					cfg.RebindTime = rebindTime
				}
				if cfg.RebindTime <= cfg.RenewTime {
					log.Infof(tag+" %s: Rebind time %v <= renew time %v, using default %v", nicName, cfg.RebindTime, cfg.RenewTime, rebindTime)
					cfg.RebindTime = rebindTime
				}
			}

			if info.State == initSelecting {
				log.Infof(tag+" %s: Performing duplicate address detection", nicName)
				if err := func() error {
					ch := make(chan stack.DADResult, 1)
					addr := info.Acquired.Address
					// Per RFC 2131 section 3.1:
					//
					//  5. The client receives the DHCPACK message with configuration
					//     parameters.  The client SHOULD perform a final check on the
					//     parameters (e.g., ARP for allocated network address), and notes the
					//     duration of the lease specified in the DHCPACK message.  At this
					//     point, the client is configured.  If the client detects that the
					//     address is already in use (e.g., through the use of ARP), the
					//     client MUST send a DHCPDECLINE message to the server and restarts
					//     the configuration process.  The client SHOULD wait a minimum of ten
					//     seconds before restarting the configuration process to avoid
					//     excessive network traffic in case of looping.
					//
					// Per RFC 2131 section 4.4.1:
					//
					// The client SHOULD perform a check on the suggested address to
					// ensure that the address is not already in use.  For example, if
					// the client is on a network that supports ARP, the client may issue
					// an ARP request for the suggested request.  When broadcasting an
					// ARP request for the suggested address, the client must fill in its
					// own hardware address as the sender's hardware address, and 0 as
					// the sender's IP address, to avoid confusing ARP caches in other
					// hosts on the same subnet.  If the network address appears to be in
					// use, the client MUST send a DHCPDECLINE message to the server. The
					// client SHOULD broadcast an ARP reply to announce the client's new
					// IP address and clear any outdated ARP cache entries in hosts on
					// the client's subnet.
					res, err := c.stack.CheckDuplicateAddress(info.NICID, header.IPv4ProtocolNumber, addr, func(result stack.DADResult) {
						ch <- result
					})
					switch err.(type) {
					case nil:
					case *tcpip.ErrNotSupported:
						log.Infof(tag+" %s: DAD not supported, proceeding with address", nicName)
						// If the link does not support DAD, then we have no way to check if
						// the address is already in use by a neighbor so be optimistic and
						// proceed with the acquired address.
						return nil
					default:
						log.Infof(tag+" %s: Failed to start DAD: %v", nicName, err)
						return fmt.Errorf("failed to start duplicate address detection on %s: %s", addr, err)
					}
					switch res {
					case stack.DADStarting, stack.DADAlreadyRunning:
						log.Infof(tag+" %s: DAD started", nicName)
					case stack.DADDisabled:
						log.Infof(tag+" %s: DAD disabled, proceeding with address", nicName)
						// If the stack is not configured to perform DAD, then we have no
						// way to check if the address is already in use by a neighbor so
						// we proceed with the acquired address without checking if it is
						// already assigned to a neighbor.
						return nil
					default:
						panic(fmt.Sprintf("unexpected result = %d", res))
					}

					select {
					case <-ctx.Done():
						log.Infof(tag+" %s: DAD interrupted: %v", nicName, ctx.Err())
						return fmt.Errorf("failed to complete duplicate address detection on %s: %w", addr, ctx.Err())
					case result := <-ch:
						switch result := result.(type) {
						case *stack.DADSucceeded:
							log.Infof(tag+" %s: DAD succeeded", nicName)
							// DAD did not find a neighbor with the address assigned so we are
							// safe to proceed with the address.
							return nil
						case *stack.DADError:
							log.Infof(tag+" %s: DAD error: %v", nicName, result.Err)
							return fmt.Errorf("error performing duplicate address detection on %s: %s", addr, result.Err)
						case *stack.DADAborted:
							log.Infof(tag+" %s: DAD aborted", nicName)
							return fmt.Errorf("duplicate address detection aborted on %s", addr)
						case *stack.DADDupAddrDetected:
							log.Infof(tag+" %s: DAD detected duplicate address held by %v", nicName, result.HolderLinkAddress)
							info.Backoff = minBackoffAfterDupAddrDetetected
							// As per RFC 2131 section 4.4.1,
							//
							//  Option                     DHCPDECLINE
							//  ------                     -----------
							//  DHCP message type          DHCPDECLINE
							//
							//  Requested IP address       MUST
							//
							//  Server identifier          MUST
							//
							//  Client identifier          MAY
							if err := c.send(
								ctx,
								nicName,
								&info,
								options{
									{optDHCPMsgType, []byte{byte(dhcpDECLINE)}},
									{optReqIPAddr, []byte(addr.String())},
									{optDHCPServer, []byte(info.Config.ServerAddress.AsSlice())},
								},
								tcpip.FullAddress{
									NIC:  info.NICID,
									Addr: header.IPv4Broadcast,
									Port: ServerPort,
								},
								true,  /* broadcast */
								false, /* ciaddr */
							); err != nil {
								log.Infof(tag+" %s: Failed to send DECLINE: %v", nicName, err)
								return fmt.Errorf("%s: %w", dhcpDECLINE, err)
							}
							return fmt.Errorf("declined %s because it is held by %s", info.Acquired, result.HolderLinkAddress)
						default:
							panic(fmt.Sprintf("unhandled DAD result variant %#v", result))
						}
					}
				}(); err != nil {
					log.Infof(tag+" %s: DAD failed: %v", nicName, err)
					return fmt.Errorf("DAD: %w", err)
				}
			}

			// We successfully completed a transaction, so reset the log throttling
			// state.
			log.Infof(tag+" %s: Assigning configuration", nicName)
			c.assign(ctx, &info, info.Acquired, &cfg, txnStart)

			return nil
		}(); err != nil {
			if ctx.Err() != nil {
				log.Infof(tag+" %s: Context cancelled", nicName)
				return
			}
			log.Infof(tag+" %s: Error during acquisition: %v", nicName, err)
		}

		// Synchronize info after attempt to acquire is complete.
		c.storeInfo(&info)

		// RFC 2131 Section 4.4.5
		// https://tools.ietf.org/html/rfc2131#section-4.4.5
		//
		//   T1 MUST be earlier than T2, which, in turn, MUST be earlier than
		//   the time at which the client's lease will expire.
		var next dhcpClientState
		var waitDuration time.Duration
		switch now := c.now(); {
		case !now.Before(info.LeaseExpiration):
			next = initSelecting
			log.Infof(tag+" %s: Lease expired, transitioning to initSelecting", nicName)
		case !now.Before(info.RebindTime):
			next = rebinding
			log.Infof(tag+" %s: Rebind time reached, transitioning to rebinding", nicName)
		case !now.Before(info.RenewTime):
			next = renewing
			log.Infof(tag+" %s: Renew time reached, transitioning to renewing", nicName)
		default:
			switch s := info.State; s {
			case renewing, rebinding:
				// This means the client is stuck in a bad state, because if
				// the timers are correctly set, previous cases should have matched.
				panic(fmt.Sprintf(
					"invalid client state %s, now=%s, leaseExpirationTime=%s, renewTime=%s, rebindTime=%s",
					s, now, info.LeaseExpiration, info.RenewTime, info.RebindTime,
				))
			}
			waitDuration = info.RenewTime.Sub(now)
			next = renewing
			log.Infof(tag+" %s: Waiting %v before transitioning to renewing", nicName, waitDuration)
		}

		// No state transition occurred, the client is retrying.
		if info.State == next {
			waitDuration = info.Backoff
			log.Infof(tag+" %s: Retrying in %v", nicName, waitDuration)
		}

		if info.State != next && next != renewing {
			// Transition immediately for RENEW->REBIND, REBIND->INIT.
			if ctx.Err() != nil {
				log.Infof(tag+" %s: Context cancelled during state transition", nicName)
				return
			}
			log.Infof(tag+" %s: Transitioning from %s to %s", nicName, info.State, next)
		} else {
			select {
			case <-ctx.Done():
				log.Infof(tag+" %s: Context cancelled during wait", nicName)
				return
			case <-c.retransTimeout(waitDuration):
				log.Infof(tag+" %s: Wait complete", nicName)
			}
		}

		if info.State != initSelecting && next == initSelecting {
			log.Infof(tag+" %s: Lease expired, cleaning up", nicName)
			c.lost(ctx, c.cleanup(&info, nicName, true /* release */))
		}

		info.State = next
		log.Infof(tag+" %s: State set to %s", nicName, next)

		// Synchronize info after any state updates.
		c.storeInfo(&info)
	}
}

func (c *Client) acquired(ctx context.Context, lost, acquired tcpip.AddressWithPrefix, config Config) {
	if fn := c.acquiredFunc; fn != nil {
		fn(ctx, lost, acquired, config)
	}
}

func (c *Client) lost(ctx context.Context, lost tcpip.AddressWithPrefix) {
	c.acquired(ctx, lost, tcpip.AddressWithPrefix{}, Config{})
}

func (c *Client) assign(ctx context.Context, info *Info, acquired tcpip.AddressWithPrefix, config *Config, now time.Time) {
	prevAssigned := c.updateInfo(info, acquired, config, now, bound)
	c.acquired(ctx, prevAssigned, acquired, *config)
}

func (c *Client) updateInfo(info *Info, acquired tcpip.AddressWithPrefix, config *Config, now time.Time, state dhcpClientState) tcpip.AddressWithPrefix {
	config.UpdatedAt = now
	prevAssigned := info.Assigned
	info.Assigned = acquired
	info.LeaseExpiration = now.Add(config.LeaseLength.Duration())
	info.RenewTime = now.Add(config.RenewTime.Duration())
	info.RebindTime = now.Add(config.RebindTime.Duration())
	info.Config = *config
	info.State = state
	c.storeInfo(info)
	return prevAssigned
}

func (c *Client) cleanup(info *Info, nicName string, release bool) tcpip.AddressWithPrefix {
	if release && info.Assigned != (tcpip.AddressWithPrefix{}) {
		if err := func() error {
			// As per RFC 2131 section 4.4.1,
			//
			//  Option                     DHCPRELEASE
			//  ------                     -----------
			//  DHCP message type          DHCPRELEASE
			//
			//  Requested IP address       MUST NOT
			//
			//  Server identifier          MUST
			//
			//  Client identifier          MAY
			if err := c.send(
				context.Background(),
				nicName,
				info,
				options{
					{optDHCPMsgType, []byte{byte(dhcpRELEASE)}},
					{optDHCPServer, []byte(info.Config.ServerAddress.AsSlice())},
				},
				tcpip.FullAddress{
					Addr: info.Config.ServerAddress,
					Port: ServerPort,
					NIC:  info.NICID,
				},
				false, /* broadcast */
				true,  /* ciaddr */
			); err != nil {
				return fmt.Errorf("%s: %w", dhcpRELEASE, err)
			}
			return nil
		}(); err != nil {
			//_ = syslog.WarnTf(tag, "%s, continuing", err)
		}
	}

	return c.updateInfo(info, tcpip.AddressWithPrefix{}, &Config{}, time.Time{}, info.State)
}

const maxBackoff = 64 * time.Second

// Exponential backoff calculates the backoff delay for this iteration (0-indexed) of retransmission.
//
// RFC 2131 section 4.1
// https://tools.ietf.org/html/rfc2131#section-4.1
//
//	The delay between retransmissions SHOULD be
//	chosen to allow sufficient time for replies from the server to be
//	delivered based on the characteristics of the internetwork between
//	the client and the server.  For example, in a 10Mb/sec Ethernet
//	internetwork, the delay before the first retransmission SHOULD be 4
//	seconds randomized by the value of a uniform random number chosen
//	from the range -1 to +1.  Clients with clocks that provide resolution
//	granularity of less than one second may choose a non-integer
//	randomization value.  The delay before the next retransmission SHOULD
//	be 8 seconds randomized by the value of a uniform number chosen from
//	the range -1 to +1.  The retransmission delay SHOULD be doubled with
//	subsequent retransmissions up to a maximum of 64 seconds.
func (c *Client) exponentialBackoff(iteration uint) time.Duration {
	jitter := time.Duration(c.rand.Int63n(int64(2*time.Second+1))) - time.Second // [-1s, +1s]
	backoff := maxBackoff
	// Guards against overflow.
	if retransmission := c.Info().Retransmission; (maxBackoff/retransmission)>>iteration != 0 {
		backoff = retransmission * (1 << iteration)
	}
	backoff += jitter
	if backoff < 0 {
		return 0
	}
	return backoff
}

func acquire(ctx context.Context, c *Client, nicName string, info *Info) (Config, error) {
	// https://tools.ietf.org/html/rfc2131#section-4.3.6 Client messages:
	//
	// ---------------------------------------------------------------------
	// |              |INIT-REBOOT  |SELECTING    |RENEWING     |REBINDING |
	// ---------------------------------------------------------------------
	// |broad/unicast |broadcast    |broadcast    |unicast      |broadcast |
	// |server-ip     |MUST NOT     |MUST         |MUST NOT     |MUST NOT  |
	// |requested-ip  |MUST         |MUST         |MUST NOT     |MUST NOT  |
	// |ciaddr        |zero         |zero         |IP address   |IP address|
	// ---------------------------------------------------------------------
	writeTo := tcpip.FullAddress{
		Addr: header.IPv4Broadcast,
		Port: ServerPort,
		NIC:  info.NICID,
	}

	// We pass the zero value network protocol to make sure we don't receive
	// packets so that we can bind to IPv4 on the interface we are interested in.
	//
	// This prevents us from receiving packets that arrive on interfaces different
	// from the interface the client is performing DHCP on.
	netProto := udp.NewProtocol(c.stack)
	ep, err := netProto.NewEndpoint(header.IPv4ProtocolNumber, &c.wq)
	if err != nil {
		return Config{}, fmt.Errorf("udp.NewProtocol.NewEndpoint: %s", err)
	}
	defer ep.Close()

	recvOn := tcpip.FullAddress{
		NIC:  info.NICID,
		Port: ClientPort,
		Addr: tcpip.Address(tcpip.AddrFrom4([4]byte{0, 0, 0, 0})),
	}
	if err := ep.Bind(recvOn); err != nil {
		return Config{}, fmt.Errorf("ep.Bind(%+v): %s", recvOn, err)
	}

	switch info.State {
	case initSelecting:
	case renewing:
		writeTo.Addr = info.Config.ServerAddress
	case rebinding:
	default:
		panic(fmt.Sprintf("unknown client state: c.State=%s", info.State))
	}

	we, ch := waiter.NewChannelEntry(waiter.EventIn)
	c.wq.EventRegister(&we)
	defer c.wq.EventUnregister(&we)

	if _, err := c.rand.Read(c.xid[:]); err != nil {
		return Config{}, fmt.Errorf("c.rand.Read(): %w", err)
	}

	commonOpts := options{
		{optParamReq, []byte{
			1,  // request subnet mask
			3,  // request router
			15, // domain name
			6,  // domain name server
		}},
	}
	log.Infof("DHCP : server address: %v", info.Config.ServerAddress.String())
	requestedAddr := info.Acquired
	if info.State == initSelecting {
		discOpts := append(options{
			{optDHCPMsgType, []byte{byte(dhcpDISCOVER)}},
			// {optDHCPServer, []byte(info.Config.ServerAddress.AsSlice())},
		}, commonOpts...)
		if requestedAddr.Address.Len() != 0 {
			discOpts = append(discOpts, option{optReqIPAddr, []byte(requestedAddr.Address.AsSlice())})
		}

	retransmitDiscover:
		for i := uint(0); ; i++ {
			log.Infof(tag+" %s: Sending dhcpDISCOVER", nicName)
			if err := c.send(
				ctx,
				nicName,
				info,
				discOpts,
				writeTo,
				true,  /* broadcast */
				false, /* ciaddr */
			); err != nil {
				c.stats.SendDiscoverErrors.Increment()
				return Config{}, fmt.Errorf("%s: %w", dhcpDISCOVER, err)
			}
			c.stats.SendDiscovers.Increment()

			// Receive a DHCPOFFER message from a responding DHCP server.
			log.Info("DHCP: Waiting for dhcpOFFER")
			retransmit := c.retransTimeout(c.exponentialBackoff(i))
			for {
				result, retransmit, err := c.recv(ctx, nicName, ep, ch, retransmit)
				if err != nil {
					if retransmit {
						c.stats.RecvOfferAcquisitionTimeout.Increment()
					} else {
						c.stats.RecvOfferErrors.Increment()
					}
					log.Infof("DHCP: Error receiving dhcpOFFER: %v", err)
					return Config{}, fmt.Errorf("recv %s: %w", dhcpOFFER, err)
				}
				if retransmit {
					c.stats.RecvOfferTimeout.Increment()
					log.Infof(tag, "%s: recv timeout waiting for %s; retransmitting %s", nicName, dhcpOFFER, dhcpDISCOVER)
					continue retransmitDiscover
				}

				if result.typ != dhcpOFFER {
					c.stats.RecvOfferUnexpectedType.Increment()
					log.Infof(tag, "%s: got DHCP type = %s from %s, want = %s; discarding", nicName, result.typ, result.source, dhcpOFFER)
					continue
				}
				c.stats.RecvOffers.Increment()

				var cfg Config
				if err := cfg.decode(result.options); err != nil {
					c.stats.RecvOfferOptsDecodeErrors.Increment()
					//_ = syslog.WarnTf(tag, "error decoding %s options: %s; discarding", result.typ, err)
					continue
				}

				if cfg.ServerAddress.Unspecified() {
					c.stats.RecvOfferNoServerAddress.Increment()
					//_ = syslog.WarnTf(tag, "%s: got %s from %s with no ServerAddress; discarding", nicName, dhcpOFFER, result.source)
					continue
				}

				if cfg.SubnetMask.Len() == 0 {
					cfg.SubnetMask = tcpip.MaskFromBytes(net.IP(result.yiaddr.AsSlice()).DefaultMask())
				}

				// We do not perform sophisticated offer selection and instead merely
				// select the first valid offer we receive.
				info.Config = cfg

				prefixLen, _ := net.IPMask(info.Config.SubnetMask.AsSlice()).Size()
				requestedAddr = tcpip.AddressWithPrefix{
					Address:   result.yiaddr,
					PrefixLen: prefixLen,
				}

				log.Infof(
					tag+" "+
						"%s: got %s from %s: Address=%s, server=%s, leaseLength=%s, renewTime=%s, rebindTime=%s",
					nicName,
					result.typ,
					result.source,
					requestedAddr,
					info.Config.ServerAddress,
					info.Config.LeaseLength,
					info.Config.RenewTime,
					info.Config.RebindTime,
				)

				break retransmitDiscover
			}
		}
	}

	reqOpts := append(options{
		{optDHCPMsgType, []byte{byte(dhcpREQUEST)}},
	}, commonOpts...)
	if info.State == initSelecting {
		reqOpts = append(reqOpts,
			options{
				{optDHCPServer, []byte(info.Config.ServerAddress.AsSlice())},
				{optReqIPAddr, []byte(requestedAddr.Address.AsSlice())},
			}...)
	}

retransmitRequest:
	for i := uint(0); ; i++ {
		if err := c.send(
			ctx,
			nicName,
			info,
			reqOpts,
			writeTo,
			true,                        /* broadcast */
			info.State != initSelecting, /* ciaddr */
		); err != nil {
			c.stats.SendRequestErrors.Increment()
			return Config{}, fmt.Errorf("%s: %w", dhcpREQUEST, err)
		}
		c.stats.SendRequests.Increment()

		// RFC 2131 Section 4.4.5
		// https://tools.ietf.org/html/rfc2131#section-4.4.5
		//
		//   In both RENEWING and REBINDING states, if the client receives no
		//   response to its DHCPREQUEST message, the client SHOULD wait one-half of
		//   the remaining time until T2 (in RENEWING state) and one-half of the
		//   remaining lease time (in REBINDING state), down to a minimum of 60
		//   seconds, before retransmitting the DHCPREQUEST message.
		var retransmitAfter time.Duration
		switch info.State {
		case initSelecting:
			retransmitAfter = c.exponentialBackoff(i)
		case renewing:
			retransmitAfter = info.RebindTime.Sub(c.now()) / 2
			if min := 60 * time.Second; retransmitAfter < min {
				retransmitAfter = min
			}
		case rebinding:
			retransmitAfter = info.LeaseExpiration.Sub(c.now()) / 2
			if min := 60 * time.Second; retransmitAfter < min {
				retransmitAfter = min
			}
		default:
			panic(fmt.Sprintf("invalid client state %s", info.State))
		}

		// Receive a DHCPACK/DHCPNAK from the server.
		retransmit := c.retransTimeout(retransmitAfter)
		for {
			result, retransmit, err := c.recv(ctx, nicName, ep, ch, retransmit)
			if err != nil {
				if retransmit {
					c.stats.RecvAckAcquisitionTimeout.Increment()
				} else {
					c.stats.RecvAckErrors.Increment()
				}
				return Config{}, fmt.Errorf("recv %s: %w", dhcpACK, err)
			}
			if retransmit {
				c.stats.RecvAckTimeout.Increment()
				//_ = syslog.WarnTf(tag, "%s: recv timeout waiting for %s; retransmitting %s", nicName, dhcpACK, dhcpREQUEST)
				continue retransmitRequest
			}

			switch result.typ {
			case dhcpACK:
				var cfg Config
				if err := cfg.decode(result.options); err != nil {
					c.stats.RecvAckOptsDecodeErrors.Increment()
					return Config{}, fmt.Errorf("%s decode: %w", result.typ, err)
				}
				prefixLen, _ := net.IPMask(cfg.SubnetMask.AsSlice()).Size()
				if addr := (tcpip.AddressWithPrefix{
					Address:   result.yiaddr,
					PrefixLen: prefixLen,
				}); addr != requestedAddr {
					c.stats.RecvAckAddrErrors.Increment()
					return Config{}, fmt.Errorf("%s with unexpected address=%s expected=%s", result.typ, addr, requestedAddr)
				}
				c.stats.RecvAcks.Increment()

				// According to RFC2132, the Server Identifier option may be omitted from DHCPACK messages
				// (https://www.rfc-editor.org/rfc/rfc2132#section-9.7). This is inconsistent with RFC2131
				// (https://www.rfc-editor.org/rfc/rfc2131#section-4.3.1), which states that DHCPACK
				// messages MUST include the Server Identifier option. Due to this inconsistency, we behave
				// permissively by using the Server Identifier we must have received in a past DHCPOFFER as
				// a fallback.
				if cfg.ServerAddress.Unspecified() {
					log.Infof(tag, "%s omits Server Identifier; continuing to use %s instead", result.typ, info.Config.ServerAddress)
					cfg.ServerAddress = info.Config.ServerAddress
				}

				// Now that we've successfully acquired the address, update the client state.
				info.Acquired = requestedAddr
				log.Infof(tag+" "+
					"%s: got %s from %s with leaseLength=%s",
					nicName,
					result.typ,
					result.source,
					cfg.LeaseLength,
				)
				return cfg, nil
			case dhcpNAK:
				c.stats.RecvNaks.Increment()
				if msg := result.options.message(); len(msg) != 0 {
					log.Infof(tag, "%s: got %s from %s (%s)", nicName, result.typ, result.source, msg)
				} else {
					log.Infof(tag, "%s: got %s from %s", nicName, result.typ, result.source)
				}
				// We lost the lease.
				return Config{
					Declined: true,
				}, nil
			default:
				c.stats.RecvAckUnexpectedType.Increment()
				log.Infof(tag, "%s: got DHCP type = %s from %s, want = %s or %s; discarding", nicName, result.typ, result.source, dhcpACK, dhcpNAK)
				continue
			}
		}
	}
}

func (c *Client) send(
	ctx context.Context,
	nicName string,
	info *Info,
	opts options,
	writeTo tcpip.FullAddress,
	broadcast,
	ciaddr bool,
) error {
	dhcpLength := headerBaseSize + opts.len() + 1
	bytes := make([]byte, header.IPv4MinimumSize+header.UDPMinimumSize+dhcpLength)
	dhcpPayload := hdr(bytes[header.IPv4MinimumSize+header.UDPMinimumSize:][:dhcpLength])
	dhcpPayload.init()
	dhcpPayload.setOp(opRequest)
	if n, l := copy(dhcpPayload.xidbytes(), c.xid[:]), len(c.xid); n != l {
		panic(fmt.Sprintf("failed to copy xid bytes, want=%d got=%d", l, n))
	}
	if broadcast {
		dhcpPayload.setBroadcast()
	}
	if ciaddr {
		ciaddr := info.Assigned.Address
		if n, l := copy(dhcpPayload.ciaddr(), ciaddr.AsSlice()), len(ciaddr.AsSlice()); n != l {
			panic(fmt.Sprintf("failed to copy ciaddr bytes, want=%d got=%d", l, n))
		}
	}

	var chaddr tcpip.LinkAddress
	if c.overrideLinkAddr == "" {
		chaddr = info.LinkAddr
	} else {
		chaddr = tcpip.LinkAddress(c.overrideLinkAddr)
	}

	if n, l := copy(dhcpPayload.chaddr(), chaddr), len(chaddr); n != l {
		log.Warn(fmt.Sprintf("failed to copy chaddr bytes, want=%d got=%d", l, n))
	}
	dhcpPayload.setOptions(opts)

	typ, err := opts.dhcpMsgType()
	if err != nil {
		_ = typ
		return utils.Error(fmt.Sprintf("failed to get DHCP message type: %s", err))
	}

	//_ = c.logThrottler.logTf(syslog.InfoLevel, tag,
	//	// Note: `broadcast_flag` here records the value of a directive to the DHCP
	//	// server (see RFC 2131, section 4.1) and does NOT imply that the `to`
	//	// address is itself a broadcast address.
	//	"%s: send %s from %s:%d to %s:%d on NIC:%d (broadcast_flag=%t ciaddr=%t)",
	//	nicName,
	//	typ,
	//	info.Assigned.Address,
	//	ClientPort,
	//	writeTo.Addr,
	//	writeTo.Port,
	//	writeTo.NIC,
	//	broadcast,
	//	ciaddr)

	// TODO(https://gvisor.dev/issues/4957): Use more streamlined serialization
	// functions when available.

	// Initialize the UDP header.
	udp := header.UDP(bytes[header.IPv4MinimumSize:][:header.UDPMinimumSize])
	length := uint16(header.UDPMinimumSize + dhcpLength)
	udp.Encode(&header.UDPFields{
		SrcPort: ClientPort,
		DstPort: writeTo.Port,
		Length:  length,
	})
	xsum := header.PseudoHeaderChecksum(header.UDPProtocolNumber, info.Assigned.Address, writeTo.Addr, length)
	xsum = checksum.Checksum(dhcpPayload, xsum)
	udp.SetChecksum(^udp.CalculateChecksum(xsum))

	// Initialize the IP header.
	ip := header.IPv4(bytes[:header.IPv4MinimumSize])
	ip.Encode(&header.IPv4Fields{
		TotalLength: uint16(len(bytes)),
		Flags:       header.IPv4FlagDontFragment,
		ID:          0,
		TTL:         c.networkEndpoint.DefaultTTL(),
		TOS:         stack.DefaultTOS,
		Protocol:    uint8(header.UDPProtocolNumber),
		SrcAddr:     info.Assigned.Address,
		DstAddr:     writeTo.Addr,
	})
	ip.SetChecksum(^ip.CalculateChecksum())

	// var linkAddress tcpip.LinkAddress
	// {
	// 	ch := make(chan stack.LinkResolutionResult, 1)
	// 	err := c.stack.GetLinkAddress(info.NICID, writeTo.Addr, info.Assigned.Address, header.IPv4ProtocolNumber, func(result stack.LinkResolutionResult) {
	// 		ch <- result
	// 	})
	// 	switch err.(type) {
	// 	case nil:
	// 		result := <-ch
	// 		linkAddress = result.LinkAddress
	// 		err = result.Err
	// 	case *tcpip.ErrWouldBlock:
	// 		select {
	// 		case <-ctx.Done():
	// 			return fmt.Errorf("client address resolution: %w", ctx.Err())
	// 		case result := <-ch:
	// 			linkAddress = result.LinkAddress
	// 			err = result.Err
	// 		}
	// 	}
	// 	if err != nil {
	// 		return fmt.Errorf("failed to resolve link address: %s", err)
	// 	}
	// }

	if err := c.stack.WritePacketToRemote(
		writeTo.NIC,
		"",
		header.IPv4ProtocolNumber,
		bufferv2.MakeWithData(bytes),
	); err != nil {
		return fmt.Errorf("failed to write packet: %s", err)
	}
	return nil
}

type recvResult struct {
	source  tcpip.Address
	yiaddr  tcpip.Address
	options options
	typ     dhcpMsgType
}

func (c *Client) recv(
	ctx context.Context,
	nicName string,
	ep tcpip.Endpoint,
	read <-chan struct{},
	retransmit <-chan time.Time,
) (recvResult, bool, error) {
	var b bytes.Buffer
	for {
		b.Reset()

		res, err := ep.Read(&b, tcpip.ReadOptions{
			NeedRemoteAddr:     true,
			NeedLinkPacketInfo: true,
		})
		// senderAddr := tcpip.LinkAddress(res.RemoteAddr.Addr.AsSlice())
		if _, ok := err.(*tcpip.ErrWouldBlock); ok {
			select {
			case <-read:
				continue
			case <-retransmit:
				return recvResult{}, true, nil
			case <-ctx.Done():
				return recvResult{}, true, fmt.Errorf("read: %w", ctx.Err())
			}
		}
		if err != nil {
			return recvResult{}, false, fmt.Errorf("read: %s", err)
		}

		// if res.LinkPacketInfo.Protocol != header.IPv4ProtocolNumber {
		// 	panic(fmt.Sprintf("received packet with non-IPv4 network protocol number: %d", header.IPv4ProtocolNumber))
		// }
		// log.Infof("recv result: %+v", res)

		// switch res.LinkPacketInfo.PktType {
		// case tcpip.PacketHost, tcpip.PacketBroadcast:
		// default:
		// 	c.stats.PacketDiscardStats.InvalidPacketType.Increment(uint64(res.LinkPacketInfo.PktType))
		// 	//_ = syslog.DebugTf(
		// 	//	tag,
		// 	//		"PacketDiscardStats.InvalidPacketType[%d]++",
		// 	//		res.LinkPacketInfo.PktType,
		// 	//)
		// 	continue
		// }

		v := b.Bytes()
		// spew.Dump(v)
		// ip := header.IPv4(v)
		// if !ip.IsValid(len(v)) {
		// 	//_ = syslog.WarnTf(
		// 	//	tag,
		// 	//	"%s: received malformed IP frame from %s; discarding %d bytes",
		// 	//	nicName,
		// 	//	senderAddr,
		// 	//	len(v),
		// 	//)
		// 	continue
		// }
		// if !ip.IsChecksumValid() {
		// 	//_ = syslog.WarnTf(
		// 	//	tag,
		// 	//	"%s: received damaged IP frame from %s; discarding %d bytes",
		// 	//	nicName,
		// 	//	senderAddr,
		// 	//	len(v),
		// 	//)
		// 	continue
		// }
		// if ip.More() || ip.FragmentOffset() != 0 {
		// 	//_ = syslog.WarnTf(
		// 	//	tag,
		// 	//	"%s: received fragmented IP frame from %s; discarding %d bytes",
		// 	//	nicName,
		// 	//	senderAddr,
		// 	//	len(v),
		// 	//)
		// 	continue
		// }
		// if ip.TransportProtocol() != header.UDPProtocolNumber {
		// 	c.stats.PacketDiscardStats.InvalidTransProto.Increment(uint64(ip.TransportProtocol()))
		// 	//_ = syslog.DebugTf(
		// 	//	tag,
		// 	//	"PacketDiscardStats.InvalidTransProto[%d]++",
		// 	//	ip.TransportProtocol(),
		// 	//)
		// 	continue
		// }
		// udp := header.UDP(ip.Payload())
		// if len(udp) < header.UDPMinimumSize {
		// 	//_ = syslog.WarnTf(
		// 	//	tag,
		// 	//		"%s: discarding malformed UDP frame (%s@%s -> %s) with length (%d) < minimum UDP size (%d)",
		// 	//		nicName,
		// 	//		ip.SourceAddress(),
		// 	//		senderAddr,
		// 	//		ip.DestinationAddress(),
		// 	//		len(udp),
		// 	//		header.UDPMinimumSize,
		// 	//)
		// 	continue
		// }
		// if udp.DestinationPort() != ClientPort {
		// 	c.stats.PacketDiscardStats.InvalidPort.Increment(uint64(udp.DestinationPort()))
		// 	////_ = syslog.DebugTf(
		// 	//	tag,
		// 	//	"PacketDiscardStats.InvalidPort[%d]++",
		// 	//	udp.DestinationPort(),
		// 	//)
		// 	continue
		// }
		// if udp.Length() > uint16(len(udp)) {
		// 	//_ = syslog.WarnTf(
		// 	//	tag,
		// 	//	"%s: discarding malformed UDP frame (%s@%s -> %s) with length (%d) < the header-specified length (%d)",
		// 	//	nicName,
		// 	//	ip.SourceAddress(),
		// 	//	senderAddr,
		// 	//	ip.DestinationAddress(),
		// 	//	len(udp),
		// 	//	udp.Length(),
		// 	//)
		// 	continue
		// }
		// payload := udp.Payload()
		// if xsum := udp.Checksum(); xsum != 0 {
		// 	if !udp.IsChecksumValid(ip.SourceAddress(), ip.DestinationAddress(), checksum.Checksum(payload, 0)) {
		// 		//_ = syslog.WarnTf(
		// 		//	tag,
		// 		//	"%s: received damaged UDP frame (%s@%s -> %s); discarding %d bytes",
		// 		//	nicName,
		// 		//	ip.SourceAddress(),
		// 		//	senderAddr,
		// 		//	ip.DestinationAddress(),
		// 		//	len(udp),
		// 		//)
		// 		continue
		// 	}
		// }

		h := hdr(v)
		if !h.isValid() {
			return recvResult{}, false, fmt.Errorf("invalid hdr: %x", h)
		}

		if op := h.op(); op != opReply {
			return recvResult{}, false, fmt.Errorf("op-code=%s, want=%s", h, opReply)
		}

		if !bytes.Equal(h.xidbytes(), c.xid[:]) {
			continue
		}

		{
			opts, err := h.options()
			if err != nil {
				return recvResult{}, false, fmt.Errorf("invalid options: %w", err)
			}

			typ, err := opts.dhcpMsgType()
			if err != nil {
				return recvResult{}, false, fmt.Errorf("invalid type: %w", err)
			}

			return recvResult{
				source:  res.RemoteAddr.Addr,
				yiaddr:  tcpip.AddrFromSlice(h.yiaddr()),
				options: opts,
				typ:     typ,
			}, false, nil
		}
	}
}
