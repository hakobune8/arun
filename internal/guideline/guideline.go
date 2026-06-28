package guideline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kazyamaz200/agentos/internal/embedding"
	"github.com/kazyamaz200/agentos/internal/vector"
	"gopkg.in/yaml.v3"
)

type Guideline struct {
	ID      string   `yaml:"id" json:"id"`
	Title   string   `yaml:"title" json:"title"`
	Rule    string   `yaml:"rule" json:"rule"`
	Tags    []string `yaml:"tags" json:"tags"`
	Example string   `yaml:"example,omitempty" json:"example,omitempty"`
}

type Store struct {
	vs         vector.VectorStore
	embed      embedding.Embedder
	collection string
}

func NewStore(vs vector.VectorStore, embed embedding.Embedder) *Store {
	return &Store{
		vs:         vs,
		embed:      embed,
		collection: "agentos_guidelines",
	}
}

func (s *Store) LoadDirectory(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read guidelines dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml")) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", entry.Name(), err)
		}

		var guidelines []Guideline
		if err := yaml.Unmarshal(data, &guidelines); err != nil {
			var single Guideline
			if err2 := yaml.Unmarshal(data, &single); err2 != nil {
				return fmt.Errorf("parse %s: %w", entry.Name(), err)
			}
			guidelines = []Guideline{single}
		}

		for _, g := range guidelines {
			if err := s.Add(context.Background(), g); err != nil {
				return fmt.Errorf("add guideline %s: %w", g.ID, err)
			}
		}
	}

	return nil
}

func (s *Store) Add(ctx context.Context, g Guideline) error {
	if g.ID == "" {
		g.ID = fmt.Sprintf("gl-%d", len(g.Title))
	}

	text := g.Title + "\n" + g.Rule
	if g.Example != "" {
		text += "\nExample:\n" + g.Example
	}

	vectors, err := s.embed.Embed(ctx, []string{text})
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}

	point := vector.Point{
		ID:     g.ID,
		Vector: vectors[0],
		Payload: map[string]interface{}{
			"title":   g.Title,
			"rule":    g.Rule,
			"tags":    g.Tags,
			"example": g.Example,
		},
	}

	return s.vs.Upsert(ctx, s.collection, []vector.Point{point})
}

func (s *Store) Search(ctx context.Context, query string, limit int) ([]Guideline, error) {
	vec, err := s.embed.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}

	points, err := s.vs.Search(ctx, s.collection, vec, limit)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	var guidelines []Guideline
	for _, p := range points {
		g := Guideline{ID: p.ID}
		if title, ok := p.Payload["title"]; ok {
			g.Title = fmt.Sprintf("%v", title)
		}
		if rule, ok := p.Payload["rule"]; ok {
			g.Rule = fmt.Sprintf("%v", rule)
		}
		if example, ok := p.Payload["example"]; ok {
			g.Example = fmt.Sprintf("%v", example)
		}
		if tags, ok := p.Payload["tags"]; ok {
			if tagList, ok := tags.([]interface{}); ok {
				for _, t := range tagList {
					g.Tags = append(g.Tags, fmt.Sprintf("%v", t))
				}
			}
		}
		guidelines = append(guidelines, g)
	}

	return guidelines, nil
}
