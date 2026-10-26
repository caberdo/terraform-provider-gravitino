package resourceutil

// AssociationID builds the compound id shared by the tag and policy
// association resources: metalake.object_type.object_full_name.
func AssociationID(metalake, objectType, objectFullName string) string {
	return metalake + "." + objectType + "." + objectFullName
}

// DiffAssociationNames returns the entries to add and to remove when moving from
// current to desired. Both slices are nil when the two sets are equal, and each
// entry appears in exactly one of them, so the result can be used directly as
// the tagsToAdd/tagsToRemove (or policiesToAdd/policiesToRemove) payload.
func DiffAssociationNames(current, desired []string) (toAdd, toRemove []string) {
	currentSet := make(map[string]struct{}, len(current))
	for _, name := range current {
		currentSet[name] = struct{}{}
	}

	desiredSet := make(map[string]struct{}, len(desired))
	for _, name := range desired {
		desiredSet[name] = struct{}{}
		if _, ok := currentSet[name]; !ok {
			toAdd = append(toAdd, name)
		}
	}

	for _, name := range current {
		if _, ok := desiredSet[name]; !ok {
			toRemove = append(toRemove, name)
		}
	}

	return toAdd, toRemove
}
