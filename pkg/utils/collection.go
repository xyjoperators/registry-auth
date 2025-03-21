package utils

import (
	"k8s.io/apimachinery/pkg/util/sets"
)

func CollectionRemove[T comparable](slice []T, item T) []T {
	set := sets.New[T](slice...)
	set = set.Delete(item)
	return set.UnsortedList()
}

func CollectionFilter[T any](slice []T, filterFunc func(T) bool) []T {
	if len(slice) == 0 {
		return slice
	}
	var newSlice = make([]T, 0)
	for _, item := range slice {
		item := item
		if filterFunc(item) {
			newSlice = append(newSlice, item)
		}
	}
	return newSlice
}

func CollectionHas[T comparable](slice []T, item T) bool {
	sets := sets.New[T](slice...)
	return sets.Has(item)
}

func CollectionInsert[T comparable](slice []T, item T) []T {
	set := sets.New[T](slice...)
	if !CollectionHas[T](slice, item) {
		set.Insert(item)
	}
	return set.UnsortedList()
}

func CollectionMap[T any, R any](slice []T, cvtFunc func(T) R) []R {
	var result = make([]R, len(slice))
	for idx, item := range slice {
		result[idx] = cvtFunc(item)
	}
	return result
}

// CollectionOfDiffSet 返回差集
// 集合 T - 集合 V
func CollectionOfDiffSet[T any, V any](sliceT []T, sliceV []V, equalFunc func(t T, v V) bool) []T {
	var result []T
	for _, t := range sliceT {
		t := t
		var hasEqual bool
		for _, v := range sliceV {
			if equalFunc(t, v) {
				hasEqual = true
				break
			}
		}
		if !hasEqual {
			result = append(result, t)
		}
	}
	return result

}
