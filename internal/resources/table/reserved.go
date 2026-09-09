package table

// reservedProperties are managed by Gravitino through dedicated endpoints
// (e.g. "in-use" via PATCH) and must never be sent as regular
// setProperty/removeProperty updates or surfaced as drift.
var reservedProperties = map[string]bool{
	"in-use": true,
}

func filterReservedProperties(props map[string]string) map[string]string {
	filtered := make(map[string]string)
	for k, v := range props {
		if !reservedProperties[k] {
			filtered[k] = v
		}
	}
	return filtered
}
