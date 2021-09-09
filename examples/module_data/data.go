package module_data

type RoleInfo struct {
	name  string
	level int32
	id    int32
}

func (r *RoleInfo) Add(a int, b int) (int, int, int) {
	return a, b, a + b
}

type TestData struct {
	name   string
	map1   map[int32]*RoleInfo
	slice1 []*RoleInfo
}

func (p *TestData) AddMapRole(id int32, r *RoleInfo) {
	p.map1[id] = r
}

func (p *TestData) GetMapRole(id int32) *RoleInfo {
	return p.map1[id]
}

func (p *TestData) AddSliceRole(r *RoleInfo) {
	p.slice1 = append(p.slice1, r)
}

func NewRole(id int32, name string, level int32) *RoleInfo {
	return &RoleInfo{
		id:    id,
		name:  name,
		level: level,
	}
}

func NewData(name string) *TestData {
	v := &TestData{
		name: name,
		map1: make(map[int32]*RoleInfo),
	}
	return v
}
