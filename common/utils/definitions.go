package utils

func AddressFamilyUint32ToString(i uint32) string {
	switch i {
	case 0:
		return "AF_UNSPEC"
	case 1:
		return "AF_UNIX/AF_LOCAL"
	//case 1: return "AF_LOCAL"
	case 2:
		return "AF_INET"
	case 3:
		return "AF_AX25"
	case 4:
		return "AF_IPX"
	case 5:
		return "AF_APPLETALK"
	case 6:
		return "AF_NETROM"
	case 7:
		return "AF_BRIDGE"
	case 8:
		return "AF_ATMPVC"
	case 9:
		return "AF_X25"
	case 10:
		return "AF_INET6"
	case 11:
		return "AF_ROSE"
	case 12:
		return "AF_DECnet"
	case 13:
		return "AF_NETBEUI"
	case 14:
		return "AF_SECURITY"
	case 15:
		return "AF_KEY"
	case 16:
		return "AF_NETLINK/AF_ROUTE"
	//case AF_NETLINK: return "AF_ROUTE"
	case 17:
		return "AF_PACKET"
	case 18:
		return "AF_ASH"
	case 19:
		return "AF_ECONET"
	case 20:
		return "AF_ATMSVC"
	case 21:
		return "AF_RDS"
	case 22:
		return "AF_SNA"
	case 23:
		return "AF_IRDA"
	case 24:
		return "AF_PPPOX"
	case 25:
		return "AF_WANPIPE"
	case 26:
		return "AF_LLC"
	case 27:
		return "AF_IB"
	case 29:
		return "AF_CAN"
	case 30:
		return "AF_TIPC"
	case 31:
		return "AF_BLUETOOTH"
	case 32:
		return "AF_IUCV"
	case 33:
		return "AF_RXRPC"
	case 34:
		return "AF_ISDN"
	case 35:
		return "AF_PHONET"
	case 36:
		return "AF_IEEE802154"
	case 37:
		return "AF_CAIF"
	case 38:
		return "AF_ALG"
	case 39:
		return "AF_NFC"
	case 40:
		return "AF_VSOCK"
	case 41:
		return "AF_MAX"
	default:
		return "UNKOWN"
	}
}

func SocketTypeUint32ToString(i uint32) string {
	switch i {
	case 1:
		return "SOCK_STREAM"
	case 2:
		return "SOCK_DGRAM"
	case 3:
		return "SOCK_RAW"
	case 4:
		return "SOCK_RDM"
	case 5:
		return "SOCK_SEQPACKET"
	case 6:
		return "SOCK_DCCP"
	case 10:
		return "SOCK_PACKET"
	default:
		return "UNKNOWN"
	}
}
