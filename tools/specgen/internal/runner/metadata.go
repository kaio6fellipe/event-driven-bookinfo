package runner

// SpecMetadata is the org-wide metadata shared by every generated spec.
// Repo-internal: services do not override these. Adjust here when the
// org/license/server URL changes.
type SpecMetadata struct {
	OrgName        string
	OrgURL         string
	OrgEmail       string
	LicenseName    string
	LicenseURL     string
	OpenAPIServer  ServerEntry
	AsyncAPIServer ServerEntry
}

// ServerEntry models one OpenAPI/AsyncAPI servers entry.
type ServerEntry struct {
	URL         string
	Description string
}

// Metadata is the constant value threaded into every Build call.
var Metadata = SpecMetadata{
	OrgName:     "bookinfo-team",
	OrgURL:      "https://github.com/kaio6fellipe/event-driven-bookinfo",
	OrgEmail:    "noreply@bookinfo.local",
	LicenseName: "Apache-2.0",
	LicenseURL:  "https://www.apache.org/licenses/LICENSE-2.0",
	OpenAPIServer: ServerEntry{
		URL:         "http://localhost:8080",
		Description: "Local k3d gateway",
	},
	AsyncAPIServer: ServerEntry{
		URL:         "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092",
		Description: "Local Kafka bootstrap",
	},
}
