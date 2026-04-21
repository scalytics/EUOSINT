package packs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type TypeSpec struct {
	Name   string `json:"name" yaml:"name"`
	Source string `json:"source" yaml:"-"`
}

type MapLayer struct {
	ID          string `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	Kind        string `json:"kind" yaml:"kind"`
	URL         string `json:"url,omitempty" yaml:"url,omitempty"`
	Attribution string `json:"attribution,omitempty" yaml:"attribution,omitempty"`
	Source      string `json:"source" yaml:"-"`
}

type Pack struct {
	Name        string     `json:"name" yaml:"name"`
	Version     string     `json:"version" yaml:"version"`
	Description string     `json:"description,omitempty" yaml:"description,omitempty"`
	Owner       string     `json:"owner,omitempty" yaml:"owner,omitempty"`
	EntityTypes []string   `json:"entity_types,omitempty" yaml:"entity_types,omitempty"`
	EdgeTypes   []string   `json:"edge_types,omitempty" yaml:"edge_types,omitempty"`
	MapLayers   []MapLayer `json:"map_layers,omitempty" yaml:"map_layers,omitempty"`
}

type Registry struct {
	Packs       []Pack     `json:"packs"`
	EntityTypes []TypeSpec `json:"entity_types"`
	EdgeTypes   []TypeSpec `json:"edge_types"`
	MapLayers   []MapLayer `json:"map_layers"`
}

var coreEntityTypes = []string{"agent", "task", "trace", "topic", "correlation", "location", "area"}
var coreEdgeTypes = []string{"sent", "responded", "spans", "mentions", "member_of", "delegated_to", "observed_at", "in_area"}
var coreMapLayers = []MapLayer{
	{
		ID:          "osm",
		Name:        "OpenStreetMap",
		Kind:        "basemap",
		URL:         "https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png",
		Attribution: "© OpenStreetMap contributors",
		Source:      "core",
	},
}

func LoadDir(root string) (Registry, error) {
	registry := Registry{
		EntityTypes: make([]TypeSpec, 0, len(coreEntityTypes)),
		EdgeTypes:   make([]TypeSpec, 0, len(coreEdgeTypes)),
		MapLayers:   append([]MapLayer{}, coreMapLayers...),
	}
	entitySeen := map[string]string{}
	edgeSeen := map[string]string{}
	for _, name := range coreEntityTypes {
		registry.EntityTypes = append(registry.EntityTypes, TypeSpec{Name: name, Source: "core"})
		entitySeen[name] = "core"
	}
	for _, name := range coreEdgeTypes {
		registry.EdgeTypes = append(registry.EdgeTypes, TypeSpec{Name: name, Source: "core"})
		edgeSeen[name] = "core"
	}

	root = strings.TrimSpace(root)
	if root == "" {
		return registry, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return registry, nil
		}
		return Registry{}, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		packPath := filepath.Join(root, entry.Name(), "pack.yaml")
		body, err := os.ReadFile(packPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return Registry{}, err
		}
		var pack Pack
		if err := yaml.Unmarshal(body, &pack); err != nil {
			return Registry{}, fmt.Errorf("load pack %s: %w", entry.Name(), err)
		}
		if strings.TrimSpace(pack.Name) == "" {
			pack.Name = entry.Name()
		}
		if err := mergePack(&registry, pack, entitySeen, edgeSeen); err != nil {
			return Registry{}, err
		}
	}
	return registry, nil
}

func mergePack(registry *Registry, pack Pack, entitySeen, edgeSeen map[string]string) error {
	source := "pack/" + pack.Name
	for _, layer := range pack.MapLayers {
		layer.Source = source
		registry.MapLayers = append(registry.MapLayers, layer)
	}
	for _, name := range pack.EntityTypes {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if owner, ok := entitySeen[name]; ok {
			return fmt.Errorf("entity type collision %q between %s and %s", name, owner, source)
		}
		entitySeen[name] = source
		registry.EntityTypes = append(registry.EntityTypes, TypeSpec{Name: name, Source: source})
	}
	for _, name := range pack.EdgeTypes {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if owner, ok := edgeSeen[name]; ok {
			return fmt.Errorf("edge type collision %q between %s and %s", name, owner, source)
		}
		edgeSeen[name] = source
		registry.EdgeTypes = append(registry.EdgeTypes, TypeSpec{Name: name, Source: source})
	}
	registry.Packs = append(registry.Packs, pack)
	return nil
}

func (r Registry) AllowsEntityType(name string) bool {
	for _, spec := range r.EntityTypes {
		if spec.Name == name {
			return true
		}
	}
	return false
}
