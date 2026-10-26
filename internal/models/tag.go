package models

// TagObjectTypes are the metadata object types that can carry a tag
// (the associateTags operation of tags.yaml, and the "What Can Carry a Tag"
// table of the Gravitino documentation). METALAKE and ROLE are deliberately
// absent: the API does not accept them. VIEW is not in the shared
// metadataObjectType enum before Gravitino 1.3.1, so the association resource
// gates VIEW on the server version; COLUMN exists in that enum since 1.3.0.
var TagObjectTypes = []string{
	ObjectTypeCatalog,
	ObjectTypeSchema,
	ObjectTypeTable,
	ObjectTypeView,
	ObjectTypeColumn,
	ObjectTypeFileset,
	ObjectTypeTopic,
	ObjectTypeModel,
	ObjectTypeFunction,
}

type Tag struct {
	Name       string            `json:"name"`
	Comment    string            `json:"comment,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
	Audit      *Audit            `json:"audit,omitempty"`
	Inherited  *bool             `json:"inherited,omitempty"`
}

type TagListResponse struct {
	Code int   `json:"code"`
	Tags []Tag `json:"tags"`
}

type TagNameListResponse struct {
	Code  int      `json:"code"`
	Names []string `json:"names"`
}

type TagResponse struct {
	Code int `json:"code"`
	Tag  Tag `json:"tag"`
}

type TagCreateRequest struct {
	Name       string            `json:"name"`
	Comment    string            `json:"comment,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

type TagUpdateRequest struct {
	Updates []interface{} `json:"updates"`
}

func NewRenameTagRequest(newName string) interface{} {
	return struct {
		Type    string `json:"@type"`
		NewName string `json:"newName"`
	}{
		Type:    "rename",
		NewName: newName,
	}
}

func NewUpdateTagCommentRequest(newComment string) interface{} {
	return struct {
		Type       string `json:"@type"`
		NewComment string `json:"newComment"`
	}{
		Type:       "updateComment",
		NewComment: newComment,
	}
}

func NewSetTagPropertyRequest(property, value string) interface{} {
	return struct {
		Type     string `json:"@type"`
		Property string `json:"property"`
		Value    string `json:"value"`
	}{
		Type:     "setProperty",
		Property: property,
		Value:    value,
	}
}

func NewRemoveTagPropertyRequest(property string) interface{} {
	return struct {
		Type     string `json:"@type"`
		Property string `json:"property"`
	}{
		Type:     "removeProperty",
		Property: property,
	}
}

// TagsAssociateRequest associates and/or disassociates tags with a metadata object
// (POST /metalakes/{metalake}/objects/{metadataObjectType}/{metadataObjectFullName}/tags).
type TagsAssociateRequest struct {
	TagsToAdd    []string `json:"tagsToAdd,omitempty"`
	TagsToRemove []string `json:"tagsToRemove,omitempty"`
}

// MetadataObject identifies a metadata object a tag is associated with.
type MetadataObject struct {
	FullName string `json:"fullName,omitempty"`
	Type     string `json:"type,omitempty"`
}

type MetadataObjectListResponse struct {
	Code            int              `json:"code"`
	MetadataObjects []MetadataObject `json:"metadataObjects"`
}
