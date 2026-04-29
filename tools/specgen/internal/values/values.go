// Package values emits deploy/<svc>/values-generated.yaml — the
// disjoint subset of cqrs.endpoints and events.exposed that specgen owns.
//
// It owns only the keys derived from spec:
//   - cqrs.endpoints.<EventName>.{method, endpoint} for POST endpoints with an EventName
//   - events.exposed.<ExposureKey>.{topic, contentType, eventTypes} for Exposed descriptors
//
// Everything else (port, broker, replica count) stays in the
// hand-edited values-local.yaml, which is deep-merged with this file at
// helm install time.
package values

import (
	"bytes"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/walker"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/yamlutil"
)

// Input holds everything Build needs to produce a values-generated.yaml.
type Input struct {
	ServiceName string
	Endpoints   []walker.EndpointInfo
	Exposed     []walker.DescriptorInfo
}

// cqrsEntry holds the fields needed to emit one cqrs.endpoints entry.
type cqrsEntry struct {
	eventName string
	method    string
	path      string
}

// exposedGroup holds the aggregated fields for one events.exposed entry.
type exposedGroup struct {
	topic       string   // Topic shared by all descriptors in the group (enforced equal)
	contentType string   // ContentType of the first descriptor (sorted by Name)
	ceTypes     []string // Union of all CETypes in the group, sorted alphabetically
}

// buildCQRSNode constructs the cqrs: YAML mapping node from endpoints that
// carry an EventName. Returns nil when no such endpoints exist.
func buildCQRSNode(endpoints []walker.EndpointInfo) *yaml.Node {
	var entries []cqrsEntry
	for _, ep := range endpoints {
		if ep.EventName == "" {
			continue
		}
		entries = append(entries, cqrsEntry{
			eventName: ep.EventName,
			method:    ep.Method,
			path:      ep.Path,
		})
	}
	if len(entries) == 0 {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].eventName < entries[j].eventName
	})

	endpointsNode := yamlutil.Mapping()
	for _, e := range entries {
		entryNode := yamlutil.Mapping()
		yamlutil.AddScalar(entryNode, "method", e.method)
		yamlutil.AddScalar(entryNode, "endpoint", e.path)
		yamlutil.AddMapping(endpointsNode, e.eventName, entryNode)
	}
	cqrsNode := yamlutil.Mapping()
	yamlutil.AddMapping(cqrsNode, "endpoints", endpointsNode)
	return cqrsNode
}

// buildExposedNode constructs the events: YAML mapping node from Exposed
// descriptors. Returns nil, nil when the slice is empty.
// Returns an error when descriptors sharing an ExposureKey disagree on Topic.
func buildExposedNode(exposed []walker.DescriptorInfo) (*yaml.Node, error) {
	if len(exposed) == 0 {
		return nil, nil
	}

	// Group descriptors by ExposureKey (fall back to Name when ExposureKey is empty).
	groupMap := map[string]*exposedGroup{}
	groupOrder := []string{}
	for _, d := range exposed {
		key := d.ExposureKey
		if key == "" {
			key = d.Name
		}
		g, exists := groupMap[key]
		if !exists {
			g = &exposedGroup{}
			groupMap[key] = g
			groupOrder = append(groupOrder, key)
		}
		g.ceTypes = append(g.ceTypes, d.CEType)
	}

	// For each group, sort descriptors by Name so ContentType is deterministic
	// (first by Name order), enforce Topic agreement, and sort CETypes alphabetically.
	for key, g := range groupMap {
		var descs []walker.DescriptorInfo
		for _, d := range exposed {
			dk := d.ExposureKey
			if dk == "" {
				dk = d.Name
			}
			if dk == key {
				descs = append(descs, d)
			}
		}
		sort.Slice(descs, func(i, j int) bool {
			return descs[i].Name < descs[j].Name
		})
		if len(descs) > 0 {
			g.topic = descs[0].Topic
			g.contentType = descs[0].ContentType
		}
		for _, d := range descs[1:] {
			if d.Topic != g.topic {
				return nil, fmt.Errorf("events.exposed.%s: descriptors disagree on Topic (%q vs %q)", key, g.topic, d.Topic)
			}
		}
		sort.Strings(g.ceTypes)
	}

	sort.Strings(groupOrder)

	exposedNode := yamlutil.Mapping()
	for _, key := range groupOrder {
		g := groupMap[key]
		entryNode := yamlutil.Mapping()
		if g.topic != "" {
			yamlutil.AddScalar(entryNode, "topic", g.topic)
		}
		yamlutil.AddScalar(entryNode, "contentType", g.contentType)

		seqNode := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, ct := range g.ceTypes {
			seqNode.Content = append(seqNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: ct},
			)
		}
		yamlutil.AddMapping(entryNode, "eventTypes", seqNode)
		yamlutil.AddMapping(exposedNode, key, entryNode)
	}
	eventsNode := yamlutil.Mapping()
	yamlutil.AddMapping(eventsNode, "exposed", exposedNode)
	return eventsNode, nil
}

// Build returns the YAML bytes of the values-generated.yaml document.
//
// cqrs.endpoints is omitted entirely when no Endpoint has an EventName.
// events.exposed is omitted entirely when Exposed is empty.
//
// Multi-descriptor grouping: descriptors sharing an ExposureKey are merged
// into one entry. ContentType is taken from the first descriptor in the group
// (sorted by Name); if descriptors in the same group have differing
// ContentTypes, the first one wins — this situation is unusual in practice.
func Build(in Input) ([]byte, error) {
	docNode := yamlutil.Mapping()

	if cqrsNode := buildCQRSNode(in.Endpoints); cqrsNode != nil {
		yamlutil.AddMapping(docNode, "cqrs", cqrsNode)
	}

	eventsNode, err := buildExposedNode(in.Exposed)
	if err != nil {
		return nil, fmt.Errorf("building events.exposed: %w", err)
	}
	if eventsNode != nil {
		yamlutil.AddMapping(docNode, "events", eventsNode)
	}

	root := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{docNode}}

	var buf bytes.Buffer
	buf.WriteString("# DO NOT EDIT — generated by tools/specgen from\n")
	if len(in.Endpoints) > 0 {
		buf.WriteString("#   services/" + in.ServiceName + "/internal/adapter/inbound/http/endpoints.go\n")
	}
	if len(in.Exposed) > 0 {
		buf.WriteString("#   services/" + in.ServiceName + "/internal/adapter/outbound/messaging/exposed.go\n")
	}
	buf.WriteString("# Run `make generate-specs` to refresh.\n")

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
