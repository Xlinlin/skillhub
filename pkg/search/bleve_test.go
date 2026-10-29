package search

import (
	"context"
	"testing"

	"github.com/cinience/skillhub/pkg/config"
)

func newTestClient(t *testing.T) *Client {
	t.Helper()
	c, err := New(config.SearchConfig{IndexPath: t.TempDir() + "/skills.bleve"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func publicSearchFilters() string {
	return "moderationStatus = approved AND isDeleted = false"
}

func TestClientSkillSearchFiltersSortAndDelete(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t)

	docs := []*SkillDocument{
		{
			ID:               "pending",
			Slug:             "pending-browser",
			Summary:          "browser automation pending",
			OwnerHandle:      "alice",
			ModerationStatus: "pending",
			Downloads:        100,
			CreatedAt:        1,
			UpdatedAt:        1,
		},
		{
			ID:               "deleted",
			Slug:             "deleted-browser",
			Summary:          "browser automation deleted",
			OwnerHandle:      "alice",
			ModerationStatus: "approved",
			IsDeleted:        true,
			Downloads:        50,
			CreatedAt:        2,
			UpdatedAt:        2,
		},
		{
			ID:               "a-low",
			Slug:             "low-browser",
			Summary:          "browser automation low",
			OwnerHandle:      "bob",
			ModerationStatus: "approved",
			Downloads:        1,
			CreatedAt:        3,
			UpdatedAt:        3,
		},
		{
			ID:               "z-high",
			Slug:             "high-browser",
			Summary:          "browser automation high",
			OwnerHandle:      "carol",
			ModerationStatus: "approved",
			Downloads:        9,
			CreatedAt:        4,
			UpdatedAt:        4,
		},
	}
	for _, doc := range docs {
		if err := c.IndexSkill(ctx, doc); err != nil {
			t.Fatalf("IndexSkill(%s): %v", doc.ID, err)
		}
	}

	result, err := c.Search(ctx, "browser", 10, 0, []string{"downloads:desc"}, publicSearchFilters())
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if result.EstimatedTotal != 2 {
		t.Fatalf("EstimatedTotal = %d, want 2; hits=%v", result.EstimatedTotal, result.Hits)
	}
	if got := result.Hits[0]["slug"]; got != "high-browser" {
		t.Fatalf("first slug = %v, want high-browser", got)
	}
	if got := result.Hits[0]["ownerHandleExact"]; got != "carol" {
		t.Fatalf("ownerHandleExact = %v, want carol", got)
	}

	if err := c.DeleteSkill(ctx, "z-high"); err != nil {
		t.Fatalf("DeleteSkill: %v", err)
	}
	result, err = c.Search(ctx, "browser", 10, 0, nil, publicSearchFilters())
	if err != nil {
		t.Fatalf("Search after delete: %v", err)
	}
	if result.EstimatedTotal != 1 || result.Hits[0]["slug"] != "low-browser" {
		t.Fatalf("after delete hits = total %d %#v, want only low-browser", result.EstimatedTotal, result.Hits)
	}
}
