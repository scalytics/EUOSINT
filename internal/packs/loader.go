package packs

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type TypeSpec struct {
	Name   string `json:"name" yaml:"name"`
	Source string `json:"source" yaml:"-"`
}

type MapLayer struct {
	ID             string   `json:"id" yaml:"id"`
	Name           string   `json:"name" yaml:"name"`
	Kind           string   `json:"kind" yaml:"kind"`
	URL            string   `json:"url,omitempty" yaml:"url,omitempty"`
	Attribution    string   `json:"attribution,omitempty" yaml:"attribution,omitempty"`
	GeometrySource string   `json:"geometry_source,omitempty" yaml:"geometry_source,omitempty"`
	EntityTypes    []string `json:"entity_types,omitempty" yaml:"entity_types,omitempty"`
	Render         string   `json:"render,omitempty" yaml:"render,omitempty"`
	LabelField     string   `json:"label_field,omitempty" yaml:"label_field,omitempty"`
	Filter         string   `json:"filter,omitempty" yaml:"filter,omitempty"`
	Source         string   `json:"source" yaml:"-"`
}

type Detector struct {
	ID                  string   `json:"id" yaml:"id"`
	Severity            string   `json:"severity" yaml:"severity"`
	Window              string   `json:"window,omitempty" yaml:"window,omitempty"`
	Query               string   `json:"query" yaml:"-"`
	ExplanationTemplate string   `json:"explanation_template,omitempty" yaml:"explanation_template,omitempty"`
	SuggestedActions    []string `json:"suggested_actions,omitempty" yaml:"suggested_actions,omitempty"`
	Source              string   `json:"source" yaml:"-"`
}

type ViewField struct {
	ID     string `json:"id" yaml:"id"`
	Label  string `json:"label,omitempty" yaml:"label,omitempty"`
	Format string `json:"format,omitempty" yaml:"format,omitempty"`
	Hidden bool   `json:"hidden,omitempty" yaml:"hidden,omitempty"`
}

type View struct {
	ID         string      `json:"id" yaml:"id"`
	EntityType string      `json:"entity_type" yaml:"entity_type"`
	Title      string      `json:"title,omitempty" yaml:"title,omitempty"`
	Fields     []ViewField `json:"fields,omitempty" yaml:"fields,omitempty"`
	Source     string      `json:"source" yaml:"-"`
}

type QueryParam struct {
	Name        string `json:"name" yaml:"name"`
	Type        string `json:"type,omitempty" yaml:"type,omitempty"`
	Label       string `json:"label,omitempty" yaml:"label,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Required    bool   `json:"required,omitempty" yaml:"required,omitempty"`
}

type QueryTemplate struct {
	ID          string       `json:"id" yaml:"id"`
	Title       string       `json:"title,omitempty" yaml:"title,omitempty"`
	Description string       `json:"description,omitempty" yaml:"description,omitempty"`
	SQL         string       `json:"sql,omitempty" yaml:"sql,omitempty"`
	Params      []QueryParam `json:"params,omitempty" yaml:"params,omitempty"`
	Source      string       `json:"source" yaml:"-"`
}

type Requires struct {
	CoreMinVersion string `json:"core_min_version,omitempty" yaml:"core_min_version,omitempty"`
}

type Pack struct {
	Name            string          `json:"name" yaml:"name"`
	Version         string          `json:"version" yaml:"version"`
	SchemaVersion   string          `json:"schema_version,omitempty" yaml:"schema_version,omitempty"`
	Description     string          `json:"description,omitempty" yaml:"description,omitempty"`
	Owner           string          `json:"owner,omitempty" yaml:"owner,omitempty"`
	Requires        Requires        `json:"requires,omitempty" yaml:"requires,omitempty"`
	EntityTypes     []string        `json:"entity_types,omitempty" yaml:"-"`
	EdgeTypes       []string        `json:"edge_types,omitempty" yaml:"-"`
	MapLayers       []MapLayer      `json:"map_layers,omitempty" yaml:"-"`
	Detectors       []Detector      `json:"detectors,omitempty" yaml:"-"`
	Views           []View          `json:"views,omitempty" yaml:"-"`
	Queries         []QueryTemplate `json:"queries,omitempty" yaml:"-"`
	ReportTemplates []string        `json:"report_templates,omitempty" yaml:"-"`
}

type Registry struct {
	Packs       []Pack          `json:"packs"`
	EntityTypes []TypeSpec      `json:"entity_types"`
	EdgeTypes   []TypeSpec      `json:"edge_types"`
	MapLayers   []MapLayer      `json:"map_layers"`
	Detectors   []Detector      `json:"detectors,omitempty"`
	Views       []View          `json:"views,omitempty"`
	Queries     []QueryTemplate `json:"queries,omitempty"`
}

type entityTypeDecl struct {
	ID                string `yaml:"id"`
	Display           string `yaml:"display,omitempty"`
	CanonicalIDFormat string `yaml:"canonical_id_format,omitempty"`
}

func (d *entityTypeDecl) UnmarshalYAML(node *yaml.Node) error {
	type raw entityTypeDecl
	if node.Kind == yaml.ScalarNode {
		d.ID = strings.TrimSpace(node.Value)
		return nil
	}
	var next raw
	if err := node.Decode(&next); err != nil {
		return err
	}
	*d = entityTypeDecl(next)
	return nil
}

type edgeTypeDecl struct {
	ID       string   `yaml:"id"`
	Display  string   `yaml:"display,omitempty"`
	SrcTypes []string `yaml:"src_types,omitempty"`
	DstTypes []string `yaml:"dst_types,omitempty"`
	Temporal bool     `yaml:"temporal,omitempty"`
}

func (d *edgeTypeDecl) UnmarshalYAML(node *yaml.Node) error {
	type raw edgeTypeDecl
	if node.Kind == yaml.ScalarNode {
		d.ID = strings.TrimSpace(node.Value)
		return nil
	}
	var next raw
	if err := node.Decode(&next); err != nil {
		return err
	}
	*d = edgeTypeDecl(next)
	return nil
}

type detectorFile struct {
	ID       string `yaml:"id"`
	Severity string `yaml:"severity"`
	Window   string `yaml:"window,omitempty"`
	Match    struct {
		Pattern string `yaml:"pattern"`
	} `yaml:"match"`
	ExplanationTemplate string   `yaml:"explanation_template,omitempty"`
	SuggestedActions    []string `yaml:"suggested_actions,omitempty"`
}

type queryFile struct {
	ID          string       `yaml:"id"`
	Title       string       `yaml:"title,omitempty"`
	Description string       `yaml:"description,omitempty"`
	SQL         string       `yaml:"sql,omitempty"`
	Query       string       `yaml:"query,omitempty"`
	Params      []QueryParam `yaml:"params,omitempty"`
}

type packFile struct {
	Name          string           `yaml:"name"`
	Version       string           `yaml:"version"`
	SchemaVersion string           `yaml:"schema_version,omitempty"`
	Description   string           `yaml:"description,omitempty"`
	Owner         string           `yaml:"owner,omitempty"`
	Requires      Requires         `yaml:"requires,omitempty"`
	EntityTypes   []entityTypeDecl `yaml:"entity_types,omitempty"`
	EdgeTypes     []edgeTypeDecl   `yaml:"edge_types,omitempty"`
	MapLayers     []MapLayer       `yaml:"map_layers,omitempty"`
}

type mapLayersFile struct {
	Layers []MapLayer `yaml:"layers"`
}

var (
	coreEntityTypes = []string{"agent", "task", "trace", "topic", "correlation", "location", "area"}
	coreEdgeTypes   = []string{"sent", "responded", "spans", "mentions", "member_of", "delegated_to", "observed_at", "in_area"}
	coreMapLayers   = []MapLayer{
		{
			ID:          "osm",
			Name:        "OpenStreetMap",
			Kind:        "basemap",
			URL:         "https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png",
			Attribution: "© OpenStreetMap contributors",
			Source:      "core",
		},
	}
	validID       = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	validSeverity = map[string]bool{"low": true, "medium": true, "high": true, "critical": true}
	validRender   = map[string]bool{"point": true, "line": true, "polygon": true, "track": true, "heatmap": true}
)

func LoadDir(root string) (Registry, error) {
	registry := Registry{
		EntityTypes: make([]TypeSpec, 0, len(coreEntityTypes)),
		EdgeTypes:   make([]TypeSpec, 0, len(coreEdgeTypes)),
		MapLayers:   append([]MapLayer{}, coreMapLayers...),
	}
	entitySeen := map[string]string{}
	edgeSeen := map[string]string{}
	packSeen := map[string]string{}
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
		pack, err := loadPack(filepath.Join(root, entry.Name()), entry.Name(), entitySeen, edgeSeen)
		if err != nil {
			return Registry{}, err
		}
		if pack == nil {
			continue
		}
		if owner, ok := packSeen[pack.Name]; ok {
			return Registry{}, fmt.Errorf("pack name collision %q between %s and %s", pack.Name, owner, filepath.Join(root, entry.Name()))
		}
		packSeen[pack.Name] = filepath.Join(root, entry.Name())
		if err := mergePack(&registry, *pack, entitySeen, edgeSeen); err != nil {
			return Registry{}, err
		}
	}
	return registry, nil
}

func loadPack(dir, fallbackName string, entitySeen, edgeSeen map[string]string) (*Pack, error) {
	body, err := os.ReadFile(filepath.Join(dir, "pack.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var raw packFile
	if err := yaml.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("load pack %s: %w", fallbackName, err)
	}
	if strings.TrimSpace(raw.Name) == "" {
		raw.Name = fallbackName
	}
	if strings.TrimSpace(raw.Name) == "" {
		return nil, fmt.Errorf("load pack %s: missing required field name", fallbackName)
	}
	if strings.TrimSpace(raw.Version) == "" {
		return nil, fmt.Errorf("load pack %s: missing required field version", raw.Name)
	}
	schemaVersion := strings.TrimSpace(raw.SchemaVersion)
	if schemaVersion == "" {
		schemaVersion = "v1"
	}
	if schemaVersion != "v1" {
		return nil, fmt.Errorf("load pack %s: unsupported schema_version %q", raw.Name, schemaVersion)
	}

	pack := &Pack{
		Name:          raw.Name,
		Version:       raw.Version,
		SchemaVersion: schemaVersion,
		Description:   raw.Description,
		Owner:         raw.Owner,
		Requires:      raw.Requires,
	}

	source := "pack/" + pack.Name
	localEntity := copySeen(entitySeen)
	localEdge := copySeen(edgeSeen)
	entityOwn := map[string]struct{}{}
	edgeOwn := map[string]struct{}{}

	for _, entity := range raw.EntityTypes {
		id := strings.TrimSpace(entity.ID)
		if err := validateTypeID(id, "entity type", pack.Name); err != nil {
			return nil, err
		}
		if owner, ok := localEntity[id]; ok {
			return nil, fmt.Errorf("entity type collision %q between %s and %s", id, owner, source)
		}
		localEntity[id] = source
		entityOwn[id] = struct{}{}
		pack.EntityTypes = append(pack.EntityTypes, id)
	}
	sort.Strings(pack.EntityTypes)

	for _, edge := range raw.EdgeTypes {
		id := strings.TrimSpace(edge.ID)
		if err := validateTypeID(id, "edge type", pack.Name); err != nil {
			return nil, err
		}
		if owner, ok := localEdge[id]; ok {
			return nil, fmt.Errorf("edge type collision %q between %s and %s", id, owner, source)
		}
		if len(edge.SrcTypes) == 0 || len(edge.DstTypes) == 0 {
			return nil, fmt.Errorf("load pack %s: edge type %q requires src_types and dst_types", pack.Name, id)
		}
		for _, src := range edge.SrcTypes {
			if _, ok := localEntity[strings.TrimSpace(src)]; !ok {
				return nil, fmt.Errorf("load pack %s: edge type %q references unknown src_type %q", pack.Name, id, src)
			}
		}
		for _, dst := range edge.DstTypes {
			if _, ok := localEntity[strings.TrimSpace(dst)]; !ok {
				return nil, fmt.Errorf("load pack %s: edge type %q references unknown dst_type %q", pack.Name, id, dst)
			}
		}
		localEdge[id] = source
		edgeOwn[id] = struct{}{}
		pack.EdgeTypes = append(pack.EdgeTypes, id)
	}
	sort.Strings(pack.EdgeTypes)

	if err := addMapLayers(pack, source, dir, raw.MapLayers, localEntity); err != nil {
		return nil, err
	}
	if err := addDetectors(pack, source, dir); err != nil {
		return nil, err
	}
	if err := addViews(pack, source, dir, localEntity); err != nil {
		return nil, err
	}
	if err := addQueries(pack, source, dir); err != nil {
		return nil, err
	}
	if err := addReports(pack, dir); err != nil {
		return nil, err
	}

	return pack, nil
}

func mergePack(registry *Registry, pack Pack, entitySeen, edgeSeen map[string]string) error {
	source := "pack/" + pack.Name
	for _, layer := range pack.MapLayers {
		registry.MapLayers = append(registry.MapLayers, layer)
	}
	for _, detector := range pack.Detectors {
		registry.Detectors = append(registry.Detectors, detector)
	}
	for _, view := range pack.Views {
		registry.Views = append(registry.Views, view)
	}
	for _, query := range pack.Queries {
		registry.Queries = append(registry.Queries, query)
	}
	for _, name := range pack.EntityTypes {
		entitySeen[name] = source
		registry.EntityTypes = append(registry.EntityTypes, TypeSpec{Name: name, Source: source})
	}
	for _, name := range pack.EdgeTypes {
		edgeSeen[name] = source
		registry.EdgeTypes = append(registry.EdgeTypes, TypeSpec{Name: name, Source: source})
	}
	registry.Packs = append(registry.Packs, pack)
	return nil
}

func addMapLayers(pack *Pack, source, dir string, inline []MapLayer, entitySeen map[string]string) error {
	layers := append([]MapLayer{}, inline...)
	path := filepath.Join(dir, "maps", "layers.yaml")
	if body, err := os.ReadFile(path); err == nil {
		var named mapLayersFile
		if err := yaml.Unmarshal(body, &named); err == nil && len(named.Layers) > 0 {
			layers = append(layers, named.Layers...)
		} else {
			var raw []MapLayer
			if err := yaml.Unmarshal(body, &raw); err != nil {
				return fmt.Errorf("load pack %s: decode %s: %w", pack.Name, filepath.Base(path), err)
			}
			layers = append(layers, raw...)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	for i := range layers {
		layer := layers[i]
		layer.ID = strings.TrimSpace(layer.ID)
		if err := validateTypeID(layer.ID, "map layer", pack.Name); err != nil {
			return err
		}
		if strings.TrimSpace(layer.Name) == "" {
			return fmt.Errorf("load pack %s: map layer %q requires name", pack.Name, layer.ID)
		}
		if strings.TrimSpace(layer.Kind) == "" {
			layer.Kind = "overlay"
		}
		switch layer.Kind {
		case "basemap":
			if strings.TrimSpace(layer.URL) == "" {
				return fmt.Errorf("load pack %s: map layer %q requires url for basemap", pack.Name, layer.ID)
			}
		case "overlay":
			if strings.TrimSpace(layer.GeometrySource) != "entity_geometry" {
				return fmt.Errorf("load pack %s: map layer %q requires geometry_source=entity_geometry", pack.Name, layer.ID)
			}
			if len(layer.EntityTypes) == 0 {
				return fmt.Errorf("load pack %s: map layer %q requires entity_types", pack.Name, layer.ID)
			}
			render := strings.TrimSpace(layer.Render)
			if !validRender[render] {
				return fmt.Errorf("load pack %s: map layer %q has unsupported render %q", pack.Name, layer.ID, layer.Render)
			}
			for _, entityType := range layer.EntityTypes {
				entityType = strings.TrimSpace(entityType)
				if _, ok := entitySeen[entityType]; !ok {
					return fmt.Errorf("load pack %s: map layer %q references unknown entity_type %q", pack.Name, layer.ID, entityType)
				}
			}
		default:
			return fmt.Errorf("load pack %s: map layer %q has unsupported kind %q", pack.Name, layer.ID, layer.Kind)
		}
		layer.Source = source
		pack.MapLayers = append(pack.MapLayers, layer)
	}
	sort.Slice(pack.MapLayers, func(i, j int) bool { return pack.MapLayers[i].ID < pack.MapLayers[j].ID })
	return nil
}

func addDetectors(pack *Pack, source, dir string) error {
	files, err := yamlFiles(filepath.Join(dir, "detectors"))
	if err != nil {
		return err
	}
	for _, path := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var raw detectorFile
		if err := yaml.Unmarshal(body, &raw); err != nil {
			return fmt.Errorf("load pack %s: decode %s: %w", pack.Name, filepath.Base(path), err)
		}
		id := strings.TrimSpace(raw.ID)
		if err := validateTypeID(id, "detector", pack.Name); err != nil {
			return err
		}
		severity := strings.ToLower(strings.TrimSpace(raw.Severity))
		if !validSeverity[severity] {
			return fmt.Errorf("load pack %s: detector %q has unsupported severity %q", pack.Name, id, raw.Severity)
		}
		query := strings.TrimSpace(raw.Match.Pattern)
		if err := validateReadOnlySQL(query, "detector", pack.Name, id); err != nil {
			return err
		}
		pack.Detectors = append(pack.Detectors, Detector{
			ID:                  id,
			Severity:            severity,
			Window:              strings.TrimSpace(raw.Window),
			Query:               query,
			ExplanationTemplate: strings.TrimSpace(raw.ExplanationTemplate),
			SuggestedActions:    cloneStrings(raw.SuggestedActions),
			Source:              source,
		})
	}
	return nil
}

func addViews(pack *Pack, source, dir string, entitySeen map[string]string) error {
	files, err := yamlFiles(filepath.Join(dir, "views"))
	if err != nil {
		return err
	}
	for _, path := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var raw View
		if err := yaml.Unmarshal(body, &raw); err != nil {
			return fmt.Errorf("load pack %s: decode %s: %w", pack.Name, filepath.Base(path), err)
		}
		id := strings.TrimSpace(raw.ID)
		if id == "" {
			id = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}
		if err := validateTypeID(id, "view", pack.Name); err != nil {
			return err
		}
		raw.ID = id
		raw.EntityType = strings.TrimSpace(raw.EntityType)
		if _, ok := entitySeen[raw.EntityType]; !ok {
			return fmt.Errorf("load pack %s: view %q references unknown entity_type %q", pack.Name, raw.ID, raw.EntityType)
		}
		raw.Source = source
		pack.Views = append(pack.Views, raw)
	}
	return nil
}

func addQueries(pack *Pack, source, dir string) error {
	files, err := yamlFiles(filepath.Join(dir, "queries"))
	if err != nil {
		return err
	}
	for _, path := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var raw queryFile
		if err := yaml.Unmarshal(body, &raw); err != nil {
			return fmt.Errorf("load pack %s: decode %s: %w", pack.Name, filepath.Base(path), err)
		}
		id := strings.TrimSpace(raw.ID)
		if err := validateTypeID(id, "query", pack.Name); err != nil {
			return err
		}
		sqlText := strings.TrimSpace(raw.SQL)
		if sqlText == "" {
			sqlText = strings.TrimSpace(raw.Query)
		}
		if err := validateReadOnlySQL(sqlText, "query", pack.Name, id); err != nil {
			return err
		}
		pack.Queries = append(pack.Queries, QueryTemplate{
			ID:          id,
			Title:       strings.TrimSpace(raw.Title),
			Description: strings.TrimSpace(raw.Description),
			SQL:         sqlText,
			Params:      raw.Params,
			Source:      source,
		})
	}
	return nil
}

func addReports(pack *Pack, dir string) error {
	entries, err := os.ReadDir(filepath.Join(dir, "reports"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".md.tmpl") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	pack.ReportTemplates = append(pack.ReportTemplates, names...)
	return nil
}

func yamlFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			files = append(files, filepath.Join(dir, name))
		}
	}
	sort.Strings(files)
	return files, nil
}

func validateTypeID(id, kind, packName string) error {
	if id == "" {
		return fmt.Errorf("load pack %s: missing required %s id", packName, kind)
	}
	if !validID.MatchString(id) {
		return fmt.Errorf("load pack %s: invalid %s id %q", packName, kind, id)
	}
	return nil
}

func validateReadOnlySQL(query, kind, packName, id string) error {
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("load pack %s: %s %q requires SQL pattern", packName, kind, id)
	}
	normalized := strings.ToLower(strings.TrimSpace(query))
	if strings.Contains(normalized, ";") {
		return fmt.Errorf("load pack %s: %s %q must be a single read-only statement", packName, kind, id)
	}
	if !(strings.HasPrefix(normalized, "select ") || strings.HasPrefix(normalized, "with ")) {
		return fmt.Errorf("load pack %s: %s %q must start with SELECT or WITH", packName, kind, id)
	}
	for _, token := range []string{" insert ", " update ", " delete ", " drop ", " alter ", " attach ", " detach ", " pragma ", " replace "} {
		if strings.Contains(" "+normalized+" ", token) {
			return fmt.Errorf("load pack %s: %s %q contains non-read-only SQL", packName, kind, id)
		}
	}
	return nil
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func copySeen(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (r Registry) AllowsEntityType(name string) bool {
	for _, spec := range r.EntityTypes {
		if spec.Name == name {
			return true
		}
	}
	return false
}
