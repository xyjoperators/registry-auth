package utils

func MapIsEmpty[K comparable, V comparable](containerMap map[K]V) bool {
	return len(containerMap) == 0
}

func MapHasKey[K comparable, V comparable](containerMap map[K]V, key K) bool {
	if MapIsEmpty[K, V](containerMap) {
		return false
	}
	_, exist := containerMap[key]
	return exist
}

func MapHasKeyAndValue[K comparable, V comparable](containerMap map[K]V, key K, value V) bool {
	if !MapHasKey[K, V](containerMap, key) {
		return false
	}
	mapValue, _ := containerMap[key]
	return mapValue == value
}

func MapPut[K comparable, V comparable](containerMap map[K]V, key K, value V) map[K]V {
	if MapIsEmpty[K, V](containerMap) {
		containerMap = make(map[K]V)
	}
	containerMap[key] = value
	return containerMap

}

func MapMerge[K comparable, V comparable](map1 map[K]V, map2 map[K]V) map[K]V {
	if map1 == nil {
		map1 = make(map[K]V)
	}
	if map2 != nil {
		for k, v := range map2 {
			map1[k] = v
		}
	}
	return map1
}
