package watchers

type WatchListener interface {
	CacheUpdated(namespace string)
}
