package main

import (
	"bufio"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/cmd/stdinsys"
	"github.com/yaklang/yaklang/common/log"
	"time"
)

func main() {
	ins := stdinsys.GetStdinSys()
	defaultReader := ins.GetDefaultStdinMirror()

	go func() {
		time.Sleep(5 * time.Second)
		log.Info("start to switch new stdin mirror")
		ins.PreventDefaultStdinMirror()
		id, reader := ins.CreateTemporaryStdinMirror()
		log.Infof("Created temporary stdin mirror with ID: %s", id)
		scanner := bufio.NewScanner(reader)
		scanner.Split(bufio.ScanLines)
		for scanner.Scan() {
			i := scanner.Text()
			fmt.Println("new stdin reader recv: ", i)
			if i == "exit" {
				log.Info("Exiting stdin mirror read loop")
				break
			}
		}
		log.Info("Exiting temporary stdin mirror read loop")
		ins.RemoveStdinMirror(id)
	}()

	var i string
	log.Info("start to check default stdin mirror")
	scanner := bufio.NewScanner(defaultReader)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		i = scanner.Text()
		fmt.Println("default recv: ", i)
		if i == "exit" {
			log.Info("Exiting stdin mirror read loop")
			break
		}
	}
	time.Sleep(10 * time.Second)
}
