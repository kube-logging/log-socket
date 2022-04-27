package slice

func RemoveFunc[S ~[]T, T any](s *S, fn func(item T) bool) {
	for i := len(*s) - 1; i >= 0; i-- {
		if fn((*s)[i]) {
			*s = append((*s)[:i], (*s)[i+1:]...)
		}
	}
}
