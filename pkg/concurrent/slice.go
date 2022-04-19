package concurrent

import "sync"

type Slice[T any] struct {
	items []T
	mutex sync.RWMutex
}

func (s *Slice[T]) Append(items ...T) {
	defer s.writeLockUnlock()()
	s.items = append(s.items, items...)
}

func (s *Slice[T]) ForEach(fn func(T)) {
	defer s.readLockUnlock()()
	for i := range s.items {
		fn(s.items[i])
	}
}

func (s *Slice[T]) ForEachPtr(fn func(*T)) {
	defer s.readLockUnlock()()
	for i := range s.items {
		fn(&s.items[i])
	}
}

func (s *Slice[T]) ForEachPtrWithIndex(fn func(int, *T)) {
	defer s.readLockUnlock()
	for i := range s.items {
		fn(i, &s.items[i])
	}
}

func (s *Slice[T]) ForEachWithIndex(fn func(int, T)) {
	defer s.readLockUnlock()
	for i := range s.items {
		fn(i, s.items[i])
	}
}

func (s *Slice[T]) Len() int {
	defer s.readLockUnlock()
	return len(s.items)
}

func (s *Slice[T]) RemoveFunc(fn func(T) bool) {
	if s.Len() == 0 {
		return
	}
	defer s.writeLockUnlock()()
	for i := len(s.items) - 1; i >= 0; i-- {
		if fn(s.items[i]) {
			s.items = append(s.items[:i], s.items[i+1:]...)
		}
	}
}

func (s *Slice[T]) readLockUnlock() func() {
	s.mutex.RLock()
	return s.mutex.RUnlock
}

func (s *Slice[T]) writeLockUnlock() func() {
	s.mutex.Lock()
	return s.mutex.Unlock
}

func RemoveItemsFromSlice[T Equatable[T]](slice *Slice[T], items ...T) {
	if len(items) == 0 {
		return
	}
	slice.RemoveFunc(func(item T) bool {
		return EqualsAny(item, items...)
	})
}

func EqualsAny[T Equatable[T]](value T, collection ...T) bool {
	for i := range collection {
		if collection[i].Equals(value) {
			return true
		}
	}
	return false
}

type Equatable[T any] interface {
	Equals(T) bool
}
