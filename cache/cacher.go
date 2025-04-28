package cache

// K is any type which supports compare operations
// type K comparable

type Cacher[T comparable, V any] interface {
	// Put adds the value to the cache, and returns a boolean to indicate whether the value
	// already existed or not. Put refreshes the underlying tracking of the cache policy
	Put(key T, value V) bool

	// Get returns the value associated with a given key and a boolean to indicate whether
	// the value was found or not
	Get(key T) (V, bool)

	// Clear resets the underlying cache to its original state
	Clear()

	// Size returns the current size of the cache. It gives details on the number of elements currently
	// present
	Size() int

	// Capacity returns the maximum allowed elements for the underlying cache
	Capacity() int
}
