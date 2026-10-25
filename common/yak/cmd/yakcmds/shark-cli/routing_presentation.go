package sharkcli

import "fmt"

func routingFieldText(protocol, key string, value any, version uint64, fallback string) string {
	n := number(value)
	label := ""
	if protocol == "hsrp" && key == "State" {
		if version == 2 {
			label = map[uint64]string{0: "Disabled", 1: "Init", 2: "Learn", 3: "Listen", 4: "Speak", 5: "Standby", 6: "Active"}[n]
		} else {
			label = map[uint64]string{0: "Initial", 1: "Learn", 2: "Listen", 4: "Speak", 8: "Standby", 16: "Active"}[n]
		}
	}
	if protocol == "hsrp" && key == "Opcode" {
		label = map[uint64]string{0: "Hello", 1: "Coup", 2: "Resign", 3: "Interface State Advertisement"}[n]
	}
	if protocol == "bfd" && key == "State" {
		label = map[uint64]string{0: "AdminDown", 1: "Down", 2: "Init", 3: "Up"}[n]
	}
	if protocol == "glbp" && (key == "Gateway State" || key == "Forwarder State") {
		label = map[uint64]string{0: "Initial", 4: "Listen", 8: "Speak", 16: "Standby", 32: "Active"}[n]
	}
	if label != "" {
		return fmt.Sprintf("%s (%d), advertised", label, n)
	}
	if protocol == "hsrp" && (key == "Hello Time" || key == "Hold Time") {
		unit := "s"
		if version == 2 {
			unit = "ms"
		}
		return fmt.Sprintf("%d %s", n, unit)
	}
	if protocol == "bfd" && (key == "Desired Min TX" || key == "Required Min RX" || key == "Required Min Echo RX") {
		return fmt.Sprintf("%d us", n)
	}
	if protocol == "glbp" && (key == "Hello Interval" || key == "Hold Interval") {
		return fmt.Sprintf("%d ms", n)
	}
	return fallback
}
