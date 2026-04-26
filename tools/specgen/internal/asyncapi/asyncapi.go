// Package asyncapi builds AsyncAPI 3.0 YAML from walker output.
package asyncapi

import (
	"bytes"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/jsonschema"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/walker"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/yamlutil"
)

// Input holds everything Build needs to produce an AsyncAPI 3.0 document.
type Input struct {
	ServiceName string
	Version     string
	Exposed     []walker.DescriptorInfo
}

// buildMessage constructs the components.messages node for a single descriptor
// and accumulates any payload schema into the provided schemas map.
func buildMessage(d walker.DescriptorInfo, schemas map[string]*jsonschema.Schema) (*yaml.Node, error) {
	msgNode := yamlutil.Mapping()
	yamlutil.AddScalar(msgNode, "name", d.Name)
	yamlutil.AddScalar(msgNode, "title", d.Name)
	if d.Description != "" {
		yamlutil.AddScalar(msgNode, "summary", d.Description)
	}
	yamlutil.AddScalar(msgNode, "contentType", d.ContentType)

	if d.PayloadType != nil {
		payloadNode := yamlutil.Mapping()
		yamlutil.AddScalar(payloadNode, "$ref", "#/components/schemas/"+d.PayloadType.Obj().Name())
		yamlutil.AddMapping(msgNode, "payload", payloadNode)

		s, err := jsonschema.SchemaFromType(d.PayloadType)
		if err != nil {
			return nil, fmt.Errorf("schema for %s payload: %w", d.Name, err)
		}
		schemas[d.PayloadType.Obj().Name()] = s
	}

	// headers — CE binding as JSONSchema const properties.
	headersNode := yamlutil.Mapping()
	yamlutil.AddScalar(headersNode, "type", "object")

	propsNode := yamlutil.Mapping()
	// Sort header property keys alphabetically.
	for _, hk := range []string{"ce-source", "ce-specversion", "ce-type"} {
		propNode := yamlutil.Mapping()
		yamlutil.AddScalar(propNode, "type", "string")
		var constVal string
		switch hk {
		case "ce-type":
			constVal = d.CEType
		case "ce-source":
			constVal = d.CESource
		case "ce-specversion":
			constVal = d.Version
		}
		yamlutil.AddScalar(propNode, "const", constVal)
		yamlutil.AddMapping(propsNode, hk, propNode)
	}
	yamlutil.AddMapping(headersNode, "properties", propsNode)
	yamlutil.AddMapping(msgNode, "headers", headersNode)
	return msgNode, nil
}

// Build returns the YAML bytes of the AsyncAPI 3.0 document.
func Build(in Input) ([]byte, error) {
	// Group descriptors by ExposureKey (fall back to Name when ExposureKey is empty).
	type group struct {
		key         string
		descriptors []walker.DescriptorInfo
	}
	groupMap := map[string]*group{}
	groupOrder := []string{}

	for _, d := range in.Exposed {
		key := d.ExposureKey
		if key == "" {
			key = d.Name
		}
		if _, exists := groupMap[key]; !exists {
			groupMap[key] = &group{key: key}
			groupOrder = append(groupOrder, key)
		}
		groupMap[key].descriptors = append(groupMap[key].descriptors, d)
	}

	// Sort group keys alphabetically for determinism.
	sort.Strings(groupOrder)

	// Collect schemas keyed by payload type name.
	schemas := map[string]*jsonschema.Schema{}

	// Build the YAML document using *yaml.Node for deterministic key ordering.
	docNode := yamlutil.Mapping()

	// asyncapi field
	yamlutil.AddScalar(docNode, "asyncapi", "3.0.0")

	// info
	infoNode := yamlutil.Mapping()
	yamlutil.AddScalar(infoNode, "title", in.ServiceName)
	yamlutil.AddScalar(infoNode, "version", in.Version)
	yamlutil.AddMapping(docNode, "info", infoNode)

	// channels — one per ExposureKey group.
	channelsNode := yamlutil.Mapping()
	for _, key := range groupOrder {
		g := groupMap[key]

		// Sort message names within the group for determinism.
		sortedDescs := make([]walker.DescriptorInfo, len(g.descriptors))
		copy(sortedDescs, g.descriptors)
		sort.Slice(sortedDescs, func(i, j int) bool {
			return sortedDescs[i].Name < sortedDescs[j].Name
		})

		channelNode := yamlutil.Mapping()
		yamlutil.AddScalar(channelNode, "address", key)

		messagesNode := yamlutil.Mapping()
		for _, d := range sortedDescs {
			msgRefNode := yamlutil.Mapping()
			yamlutil.AddScalar(msgRefNode, "$ref", "#/components/messages/"+d.Name)
			yamlutil.AddMapping(messagesNode, d.Name, msgRefNode)
		}
		yamlutil.AddMapping(channelNode, "messages", messagesNode)
		yamlutil.AddMapping(channelsNode, key, channelNode)
	}
	yamlutil.AddMapping(docNode, "channels", channelsNode)

	// operations — one send per ExposureKey group.
	operationsNode := yamlutil.Mapping()
	for _, key := range groupOrder {
		opNode := yamlutil.Mapping()
		yamlutil.AddScalar(opNode, "action", "send")

		channelRefNode := yamlutil.Mapping()
		yamlutil.AddScalar(channelRefNode, "$ref", "#/channels/"+key)
		yamlutil.AddMapping(opNode, "channel", channelRefNode)

		yamlutil.AddMapping(operationsNode, "send_"+key, opNode)
	}
	yamlutil.AddMapping(docNode, "operations", operationsNode)

	// components.messages and collect schemas.
	messagesCompNode := yamlutil.Mapping()

	// Collect all descriptors sorted by name for components.messages.
	allDescs := make([]walker.DescriptorInfo, 0, len(in.Exposed))
	for _, key := range groupOrder {
		allDescs = append(allDescs, groupMap[key].descriptors...)
	}
	sort.Slice(allDescs, func(i, j int) bool {
		return allDescs[i].Name < allDescs[j].Name
	})

	for _, d := range allDescs {
		msgNode, err := buildMessage(d, schemas)
		if err != nil {
			return nil, err
		}
		yamlutil.AddMapping(messagesCompNode, d.Name, msgNode)
	}

	// components.schemas — sorted alphabetically.
	schemaKeys := make([]string, 0, len(schemas))
	for k := range schemas {
		schemaKeys = append(schemaKeys, k)
	}
	sort.Strings(schemaKeys)

	schemasNode := yamlutil.Mapping()
	for _, k := range schemaKeys {
		sn, err := yamlutil.SchemaToNode(schemas[k])
		if err != nil {
			return nil, fmt.Errorf("encoding schema %s: %w", k, err)
		}
		yamlutil.AddMapping(schemasNode, k, sn)
	}

	componentsNode := yamlutil.Mapping()
	yamlutil.AddMapping(componentsNode, "messages", messagesCompNode)
	yamlutil.AddMapping(componentsNode, "schemas", schemasNode)
	yamlutil.AddMapping(docNode, "components", componentsNode)

	root := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{docNode}}

	var buf bytes.Buffer
	buf.WriteString("# DO NOT EDIT — generated by tools/specgen.\n")
	buf.WriteString("# Source: services/" + in.ServiceName + "/internal/adapter/outbound/kafka/exposed.go\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return nil, fmt.Errorf("encoding YAML: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("closing YAML encoder: %w", err)
	}
	return buf.Bytes(), nil
}
