package pingutil

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	_ "net/http/pprof"
	"os"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPcapxPing(t *testing.T) {
	f, err := os.Create("cpu.prof")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := pprof.StartCPUProfile(f); err != nil {
		t.Fatal(err)
	}
	defer pprof.StopCPUProfile()

	memFile, err := os.Create("mem.prof")
	if err != nil {
		t.Fatal(err)
	}
	defer memFile.Close()

	start := time.Now()
	//targets := utils.ParseStringToHosts(`183.2.172.185/24`)
	targets := utils.ParseStringToHosts(`192.168.3.4/24`)
	swg := sync.WaitGroup{}
	var count atomic.Int32
	for _, ip := range targets {
		ip := ip
		swg.Add(1)
		go func() {
			defer swg.Done()
			result, err := PcapxPing(ip, NewPingConfig())

			if err != nil {
				t.Fatal(err)
			}
			if result.Ok {
				count.Add(1)
				fmt.Println(result.IP + " is alive")
			} else {
				fmt.Printf("%s %s\n", result.IP, result.Reason)
			}
		}()
	}
	swg.Wait()
	fmt.Println("elapsed:", time.Since(start))
	fmt.Println("count:", count.Load())
	if err := pprof.WriteHeapProfile(memFile); err != nil {
		t.Fatal(err)
	}
}
