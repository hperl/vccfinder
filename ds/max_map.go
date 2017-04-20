package ds

type MaxMap struct {
	data   map[interface{}]int
	maxKey interface{}
	maxVal int
}

func NewMaxMap() *MaxMap {
	return &MaxMap{
		data:   make(map[interface{}]int),
		maxKey: "",
		maxVal: 0,
	}
}

// Add a key to the map, updating the maximum if necessary
func (m *MaxMap) Add(key string) {
	val, _ := m.data[key]
	m.data[key] = val + 1
	if m.maxVal < val+1 {
		m.maxVal = val + 1
		m.maxKey = key
	}
}

// return maximum key and value from the map
func (m *MaxMap) Max() (key interface{}, val int) {
	return m.maxKey, m.maxVal
}

func (m *MaxMap) MaxString() (key string, val int) {
	return m.maxKey.(string), m.maxVal
}
