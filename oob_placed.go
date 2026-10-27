package partial

import (
	"context"
	"slices"
	"sync"
)

type (
	// placedRegionsKey is the context key of the placedRegions of a render.
	placedRegionsKey struct{}

	// placedRegions watches the render of the target of a partial request
	// for the out-of-band regions of the target that always swap (see
	// SetAlwaysSwapOOB): those its output places, such as with
	// {{ slot }}, are in the response already; the others go out of band
	// after it.
	placedRegions struct {
		mu sync.Mutex
		// unplaced are the IDs of the regions the render has not placed
		// yet.
		unplaced []string
	}
)

// newPlacedRegions returns a placedRegions for the render of the target p,
// nil when p has no out-of-band regions that always swap.
func newPlacedRegions(p *Partial) *placedRegions {
	ids := p.alwaysSwapOOBIDs()
	if len(ids) == 0 {
		return nil
	}

	return &placedRegions{unplaced: ids}
}

// withPlacedRegions returns ctx with placed, so the renders under it note
// the partials they place; ctx itself when placed is nil.
func withPlacedRegions(ctx context.Context, placed *placedRegions) context.Context {
	if placed == nil {
		return ctx
	}

	if ctx == nil {
		ctx = defaultRenderContext()
	}

	return context.WithValue(ctx, placedRegionsKey{}, placed)
}

// placedRegionsFrom returns the placedRegions of ctx, nil when it has none.
func placedRegionsFrom(ctx context.Context) *placedRegions {
	if ctx == nil {
		return nil
	}

	placed, _ := ctx.Value(placedRegionsKey{}).(*placedRegions)

	return placed
}

// notePlaced marks the partial p placed, when the render of ctx watches for
// it.
func notePlaced(ctx context.Context, p *Partial) {
	placed := placedRegionsFrom(ctx)
	if placed == nil || p == nil {
		return
	}

	placed.mu.Lock()
	defer placed.mu.Unlock()

	placed.unplaced = slices.DeleteFunc(placed.unplaced, func(id string) bool {
		return id == p.id
	})
}

// isUnplaced reports whether the region id is one the render watched for
// and did not place.
func (placed *placedRegions) isUnplaced(id string) bool {
	if placed == nil {
		return false
	}

	placed.mu.Lock()
	defer placed.mu.Unlock()

	return slices.Contains(placed.unplaced, id)
}

// alwaysSwapOOBIDs returns the IDs of the out-of-band regions of p that
// always swap, in the order they render.
func (p *Partial) alwaysSwapOOBIDs() []string {
	if p == nil {
		return nil
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	var ids []string

	for _, id := range p.oobChildren {
		child, ok := p.children[id]
		if ok && child.alwaysSwapOOB {
			ids = append(ids, id)
		}
	}

	return ids
}
