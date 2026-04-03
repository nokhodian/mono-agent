package main

import (
	"encoding/json"
	"fmt"
	"github.com/nokhodian/mono-agent/internal/connections"
)

type PlatformInfo struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Category   string                 `json:"category"`
	ConnectVia string                 `json:"connectVia"`
	Methods    []string               `json:"methods"`
	Fields     map[string]interface{} `json:"fields"`
	IconEmoji  string                 `json:"iconEmoji"`
}

func main() {
	all := connections.All()
	fmt.Printf("Registry count: %d\n", len(all))

	result := make([]PlatformInfo, len(all))
	for i, p := range all {
		methods := make([]string, len(p.Methods))
		for j, m := range p.Methods { methods[j] = string(m) }
		fields := make(map[string]interface{})
		for method, cf := range p.Fields { fields[string(method)] = cf }
		result[i] = PlatformInfo{ID: p.ID, Name: p.Name, Category: p.Category, ConnectVia: p.ConnectVia, Methods: methods, Fields: fields, IconEmoji: p.IconEmoji}
	}

	b, err := json.Marshal(result)
	fmt.Printf("JSON err: %v\n", err)
	fmt.Printf("JSON bytes: %d\n", len(b))
	if len(b) > 0 {
		end := 400; if end > len(b) { end = len(b) }
		fmt.Printf("Sample:\n%s\n", string(b[:end]))
	}
}
