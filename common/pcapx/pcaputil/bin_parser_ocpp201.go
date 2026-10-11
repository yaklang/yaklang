package pcaputil

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

// OCA OCPP2.0.1 edition4, urn:OCPP:Cp:2:2020:3, selected request/response
// schemas only. Bounded customData remains literal; no extension execution,
// authorization, registration policy or cross-connection identity is inferred.
func ocpp201Object(p map[string]any, strings map[string]int, objects []string, required []string) error {
	for _, key := range required {
		if _, exists := p[key]; !exists {
			return fmt.Errorf("OCPP2.0.1 missing %s", key)
		}
	}
	for key, value := range p {
		if key == "customData" {
			data, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("OCPP2.0.1 customData must be an object")
			}
			vendor, ok := data["vendorId"].(string)
			if !ok || utf8.RuneCountInString(vendor) > 255 {
				return fmt.Errorf("OCPP2.0.1 customData vendorId type or length")
			}
			// Additional properties are permitted by CustomDataType, not by the
			// containing action/type. The shared decoder already charged the graph.
			continue
		}
		if limit, ok := strings[key]; ok {
			text, ok := value.(string)
			if !ok || utf8.RuneCountInString(text) > limit {
				return fmt.Errorf("OCPP2.0.1 invalid %s", key)
			}
			continue
		}
		allowed := false
		for _, name := range objects {
			if key == name {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("OCPP2.0.1 additional property %s", key)
		}
	}
	return nil
}

func ocpp201Payload(action string, response bool, p map[string]any) error {
	if !response && action == "BootNotification" {
		if err := ocpp201Object(p, map[string]int{"reason": 64}, []string{"chargingStation"}, []string{"reason", "chargingStation"}); err != nil {
			return err
		}
		switch p["reason"] {
		case "ApplicationReset", "FirmwareUpdate", "LocalReset", "PowerUp", "RemoteReset", "ScheduledReset", "Triggered", "Unknown", "Watchdog":
		default:
			return fmt.Errorf("OCPP2.0.1 invalid reason")
		}
		station, ok := p["chargingStation"].(map[string]any)
		if !ok {
			return fmt.Errorf("OCPP2.0.1 chargingStation must be an object")
		}
		if err := ocpp201Object(station, map[string]int{"model": 20, "vendorName": 50, "serialNumber": 25, "firmwareVersion": 50}, []string{"modem"}, []string{"model", "vendorName"}); err != nil {
			return err
		}
		if value, exists := station["modem"]; exists {
			modem, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("OCPP2.0.1 modem must be an object")
			}
			if err := ocpp201Object(modem, map[string]int{"iccid": 20, "imsi": 20}, nil, nil); err != nil {
				return err
			}
		}
		return nil
	}
	if !response {
		return ocpp201Object(p, nil, nil, nil)
	} // Heartbeat Request
	fields := map[string]int{"currentTime": int(^uint(0) >> 1)}
	required := []string{"currentTime"}
	objects := []string{}
	if action == "BootNotification" {
		fields["status"] = 64
		objects = []string{"interval", "statusInfo"}
		required = append(required, "interval", "status")
	}
	if err := ocpp201Object(p, fields, objects, required); err != nil {
		return err
	}
	if !ocppDate(p["currentTime"]) {
		return fmt.Errorf("OCPP2.0.1 invalid currentTime")
	}
	if action == "BootNotification" {
		if p["status"] != "Accepted" && p["status"] != "Pending" && p["status"] != "Rejected" {
			return fmt.Errorf("OCPP2.0.1 invalid status")
		}
		n, ok := p["interval"].(json.Number)
		if !ok || !semtechExactInteger(string(n)) {
			return fmt.Errorf("OCPP2.0.1 interval must be an exact integer")
		}
		if value, exists := p["statusInfo"]; exists {
			info, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("OCPP2.0.1 statusInfo must be an object")
			}
			if err := ocpp201Object(info, map[string]int{"reasonCode": 20, "additionalInfo": 512}, nil, []string{"reasonCode"}); err != nil {
				return err
			}
		}
	}
	return nil
}
