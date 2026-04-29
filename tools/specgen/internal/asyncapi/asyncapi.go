// Package asyncapi builds AsyncAPI 3.1 YAML from walker output.
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

// Input holds everything Build needs to produce an AsyncAPI 3.1 document.
type Input struct {
	ServiceName string
	Version     string
	Exposed     []walker.DescriptorInfo
	Metadata    SpecMetadata // org-wide info.* + servers; injected by runner
}

// SpecMetadata is the subset of runner.SpecMetadata the asyncapi builder needs.
// Defined here (not imported from runner) to avoid an import cycle.
type SpecMetadata struct {
	OrgName         string
	OrgURL          string
	OrgEmail        string
	LicenseName     string
	LicenseURL      string
	AsyncAPIServers map[string]ServerEntry
}

// ServerEntry mirrors runner.ServerEntry.
type ServerEntry struct {
	URL         string
	Protocol    string
	Description string
}

// groupEntry groups descriptors sharing the same ExposureKey.
type groupEntry struct {
	key         string
	descriptors []walker.DescriptorInfo
}

// buildMessage constructs the components.messages node for a single descriptor
// and accumulates any payload schema into the provided schemas map.
func buildMessage(d walker.DescriptorInfo, serviceName string, schemas map[string]*jsonschema.Schema) (*yaml.Node, error) {
	msgNode := yamlutil.Mapping()

	// summary mirrors Description for short text; AsyncAPI requires both
	// summary and description, but for our generator they are the same source.
	yamlutil.AddScalar(msgNode, "name", d.Name)
	yamlutil.AddScalar(msgNode, "title", d.Name)
	if d.Description != "" {
		yamlutil.AddScalar(msgNode, "summary", d.Description)
		yamlutil.AddScalar(msgNode, "description", d.Description)
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

	// tags — defaults to [serviceName] when descriptor.Tags is empty.
	tags := d.Tags
	if len(tags) == 0 {
		tags = []string{serviceName}
	}
	tagsAny := make([]any, 0, len(tags))
	for _, tag := range tags {
		tagsAny = append(tagsAny, map[string]any{"name": tag})
	}
	tagsNode, err := yamlutil.AnyToNode(tagsAny)
	if err != nil {
		return nil, fmt.Errorf("encoding message tags for %s: %w", d.Name, err)
	}
	yamlutil.AddMapping(msgNode, "tags", tagsNode)

	return msgNode, nil
}

// buildComponents constructs the components.messages + components.schemas node
// from an already-sorted slice of descriptors, and returns it.
func buildComponents(allDescs []walker.DescriptorInfo, serviceName string) (*yaml.Node, error) {
	schemas := map[string]*jsonschema.Schema{}
	messagesCompNode := yamlutil.Mapping()
	for _, d := range allDescs {
		msgNode, err := buildMessage(d, serviceName, schemas)
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
	return componentsNode, nil
}

// buildOperationNode constructs one operations entry for an ExposureKey group.
// tags are the per-operation tags; the caller resolves the default.
func buildOperationNode(key, serviceName string, tags []string) (*yaml.Node, error) {
	opNode := yamlutil.Mapping()
	yamlutil.AddScalar(opNode, "action", "send")
	yamlutil.AddScalar(opNode, "description", "Publish events from the "+serviceName+" service")

	opTagsAny := make([]any, 0, len(tags))
	for _, tag := range tags {
		opTagsAny = append(opTagsAny, map[string]any{"name": tag})
	}
	opTagsNode, err := yamlutil.AnyToNode(opTagsAny)
	if err != nil {
		return nil, fmt.Errorf("encoding operation tags for %s: %w", key, err)
	}
	yamlutil.AddMapping(opNode, "tags", opTagsNode)

	channelRefNode := yamlutil.Mapping()
	yamlutil.AddScalar(channelRefNode, "$ref", "#/channels/"+key)
	yamlutil.AddMapping(opNode, "channel", channelRefNode)

	return opNode, nil
}

// channelAddress returns the wire-level Kafka topic name for the channel.
// It prefers Topic from the first descriptor that has one set, falling back
// to the ExposureKey so that older services without an explicit Topic still
// produce a non-empty address.
func channelAddress(group []walker.DescriptorInfo, exposureKey string) string {
	for _, d := range group {
		if d.Topic != "" {
			return d.Topic
		}
	}
	return exposureKey
}

// buildChannelsNode builds the channels mapping from ordered groups.
func buildChannelsNode(groupOrder []string, groupMap map[string]*groupEntry) (*yaml.Node, error) {
	channelsNode := yamlutil.Mapping()
	for _, key := range groupOrder {
		g := groupMap[key]
		sortedDescs := make([]walker.DescriptorInfo, len(g.descriptors))
		copy(sortedDescs, g.descriptors)
		sort.Slice(sortedDescs, func(i, j int) bool {
			return sortedDescs[i].Name < sortedDescs[j].Name
		})

		channelNode := yamlutil.Mapping()
		yamlutil.AddScalar(channelNode, "address", channelAddress(g.descriptors, key))

		messagesNode := yamlutil.Mapping()
		for _, d := range sortedDescs {
			msgRefNode := yamlutil.Mapping()
			yamlutil.AddScalar(msgRefNode, "$ref", "#/components/messages/"+d.Name)
			yamlutil.AddMapping(messagesNode, d.Name, msgRefNode)
		}
		yamlutil.AddMapping(channelNode, "messages", messagesNode)
		yamlutil.AddMapping(channelsNode, key, channelNode)
	}
	return channelsNode, nil
}

// buildOperationsNode builds the operations mapping from ordered groups.
func buildOperationsNode(groupOrder []string, groupMap map[string]*groupEntry, serviceName string) (*yaml.Node, error) {
	operationsNode := yamlutil.Mapping()
	for _, key := range groupOrder {
		opTags := []string{serviceName}
		if g := groupMap[key]; g != nil && len(g.descriptors) > 0 && len(g.descriptors[0].Tags) > 0 {
			opTags = g.descriptors[0].Tags
		}
		opNode, err := buildOperationNode(key, serviceName, opTags)
		if err != nil {
			return nil, err
		}
		yamlutil.AddMapping(operationsNode, "send_"+key, opNode)
	}
	return operationsNode, nil
}

// buildInfoTagsNode returns a YAML sequence node of {name: tag} objects for all
// unique tag names in exposed descriptors (sorted alphabetically). Returns nil
// when the slice is empty so the caller can skip emitting the field.
func buildInfoTagsNode(exposed []walker.DescriptorInfo, serviceName string) (*yaml.Node, error) {
	tagSet := map[string]struct{}{}
	for _, d := range exposed {
		tags := d.Tags
		if len(tags) == 0 {
			tags = []string{serviceName}
		}
		for _, t := range tags {
			tagSet[t] = struct{}{}
		}
	}
	if len(tagSet) == 0 {
		return nil, nil
	}
	tagNames := make([]string, 0, len(tagSet))
	for t := range tagSet {
		tagNames = append(tagNames, t)
	}
	sort.Strings(tagNames)
	tagEntries := make([]any, 0, len(tagNames))
	for _, t := range tagNames {
		tagEntries = append(tagEntries, map[string]any{"name": t})
	}
	return yamlutil.AnyToNode(tagEntries)
}

// Build returns the YAML bytes of the AsyncAPI 3.1 document.
func Build(in Input) ([]byte, error) {
	// Group descriptors by ExposureKey (fall back to Name when ExposureKey is empty).
	groupMap := map[string]*groupEntry{}
	groupOrder := []string{}

	for _, d := range in.Exposed {
		key := d.ExposureKey
		if key == "" {
			key = d.Name
		}
		if _, exists := groupMap[key]; !exists {
			groupMap[key] = &groupEntry{key: key}
			groupOrder = append(groupOrder, key)
		}
		groupMap[key].descriptors = append(groupMap[key].descriptors, d)
	}

	// Sort group keys alphabetically for determinism.
	sort.Strings(groupOrder)

	// Build the YAML document using *yaml.Node for deterministic key ordering.
	docNode := yamlutil.Mapping()

	// asyncapi field
	yamlutil.AddScalar(docNode, "asyncapi", "3.1.0")

	// info
	infoNode := yamlutil.Mapping()
	yamlutil.AddScalar(infoNode, "title", in.ServiceName)
	yamlutil.AddScalar(infoNode, "version", in.Version)
	yamlutil.AddScalar(infoNode, "description", in.ServiceName+" — generated by tools/specgen")

	contactNode := yamlutil.Mapping()
	yamlutil.AddScalar(contactNode, "name", in.Metadata.OrgName)
	yamlutil.AddScalar(contactNode, "url", in.Metadata.OrgURL)
	yamlutil.AddScalar(contactNode, "email", in.Metadata.OrgEmail)
	yamlutil.AddMapping(infoNode, "contact", contactNode)

	licenseNode := yamlutil.Mapping()
	yamlutil.AddScalar(licenseNode, "name", in.Metadata.LicenseName)
	yamlutil.AddScalar(licenseNode, "url", in.Metadata.LicenseURL)
	yamlutil.AddMapping(infoNode, "license", licenseNode)

	// info.tags — required by spectral asyncapi-3-tags rule ($.info.tags must be truthy).
	infoTagsNode, err := buildInfoTagsNode(in.Exposed, in.ServiceName)
	if err != nil {
		return nil, fmt.Errorf("encoding info tags: %w", err)
	}
	if infoTagsNode != nil {
		yamlutil.AddMapping(infoNode, "tags", infoTagsNode)
	}

	yamlutil.AddMapping(docNode, "info", infoNode)

	// servers — AsyncAPI 3.x servers is a mapping (not a list like OpenAPI).
	// Iterate sorted by name for deterministic output (Go map iteration is randomised).
	serversNode := yamlutil.Mapping()
	serverNames := make([]string, 0, len(in.Metadata.AsyncAPIServers))
	for name := range in.Metadata.AsyncAPIServers {
		serverNames = append(serverNames, name)
	}
	sort.Strings(serverNames)
	for _, name := range serverNames {
		srv := in.Metadata.AsyncAPIServers[name]
		serverNode := yamlutil.Mapping()
		yamlutil.AddScalar(serverNode, "host", srv.URL)
		yamlutil.AddScalar(serverNode, "protocol", srv.Protocol)
		yamlutil.AddScalar(serverNode, "description", srv.Description)
		yamlutil.AddMapping(serversNode, name, serverNode)
	}
	yamlutil.AddMapping(docNode, "servers", serversNode)

	// channels — one per ExposureKey group.
	channelsNode, err := buildChannelsNode(groupOrder, groupMap)
	if err != nil {
		return nil, err
	}
	yamlutil.AddMapping(docNode, "channels", channelsNode)

	// operations — one send per ExposureKey group.
	operationsNode, err := buildOperationsNode(groupOrder, groupMap, in.ServiceName)
	if err != nil {
		return nil, err
	}
	yamlutil.AddMapping(docNode, "operations", operationsNode)

	// components — collect all descriptors sorted by name, then build.
	allDescs := make([]walker.DescriptorInfo, 0, len(in.Exposed))
	for _, key := range groupOrder {
		allDescs = append(allDescs, groupMap[key].descriptors...)
	}
	sort.Slice(allDescs, func(i, j int) bool {
		return allDescs[i].Name < allDescs[j].Name
	})

	componentsNode, err := buildComponents(allDescs, in.ServiceName)
	if err != nil {
		return nil, err
	}
	yamlutil.AddMapping(docNode, "components", componentsNode)

	root := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{docNode}}

	var buf bytes.Buffer
	buf.WriteString("# DO NOT EDIT — generated by tools/specgen.\n")
	buf.WriteString("# Source: services/" + in.ServiceName + "/internal/adapter/outbound/messaging/exposed.go\n")
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
