package resource

type Template struct {
	Key map[string]Resource
}

type Resource struct {
	namespace  string
	group      string
	version    string
	definition map[string]Definition
}

// Definition represents a resource definition
type Definition struct {
	NameFormat  string
	Resource    string
	StatusField map[interface{}]interface{}
	Kind        string
}
