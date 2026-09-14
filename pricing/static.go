package pricing

// StaticResolver resolves pricing from a map. Safe for concurrent use
// because the constructor copies the input map and the resolver never
// mutates it.
type StaticResolver struct {
	prices map[ModelPricingKey]Price
}

// NewStaticResolver creates a StaticResolver from the given price map.
// The constructor copies the map so subsequent mutations to the caller's
// map do not affect the resolver.
func NewStaticResolver(prices map[ModelPricingKey]Price) *StaticResolver {
	copied := make(map[ModelPricingKey]Price, len(prices))
	for k, v := range prices {
		copied[k] = v
	}
	return &StaticResolver{prices: copied}
}

// Resolve returns the price for the given key, or (Price{}, false) if
// the key is not found.
func (r *StaticResolver) Resolve(key ModelPricingKey) (Price, bool) {
	p, ok := r.prices[key]
	return p, ok
}
