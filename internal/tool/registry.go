package tool

import "fmt"

type Registry struct {
	tools map[string]Executor
	order []string
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]Executor{}}
}

func DefaultRegistry() (*Registry, error) {
	registry := NewRegistry()
	for _, tool := range []Executor{
		ReadFile{},
		WriteFile{},
		EditFile{},
		RunCommand{},
		FindFiles{},
		SearchCode{},
	} {
		if err := registry.Register(tool); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (r *Registry) Register(tool Executor) error {
	def := tool.Definition()
	if def.Name == "" {
		return fmt.Errorf("tool name is required")
	}
	if _, exists := r.tools[def.Name]; exists {
		return fmt.Errorf("tool already registered: %s", def.Name)
	}
	r.tools[def.Name] = tool
	r.order = append(r.order, def.Name)
	return nil
}

func (r *Registry) Get(name string) (Executor, error) {
	tool, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	return tool, nil
}

func (r *Registry) Definitions() []Definition {
	defs := make([]Definition, 0, len(r.order))
	for _, name := range r.order {
		defs = append(defs, r.tools[name].Definition())
	}
	return defs
}

func (r *Registry) CloneFiltered(defs []Definition) *Registry {
	allowed := map[string]bool{}
	for _, def := range defs {
		allowed[def.Name] = true
	}
	clone := NewRegistry()
	for _, name := range r.order {
		if allowed[name] {
			clone.tools[name] = r.tools[name]
			clone.order = append(clone.order, name)
		}
	}
	return clone
}
