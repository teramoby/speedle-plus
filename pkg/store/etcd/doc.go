// Package etcd implements an etcd-backed policy store.
// It persists policy data in an etcd cluster and supports
// real-time watch notifications so that policy changes are
// propagated to all connected ADS and PMS instances.
package etcd
