package module_data

func testAdd(a, b int) (int, int, int) {
	return a, b, a + b
}

type RoleInfo struct {
	name  string
	level int32
	id    int32
}

func (r *RoleInfo) Add(a int, b int) (int, int, int) {
	return testAdd(a, b)
}

func (r *RoleInfo) setName(name string) {
	r.name = name
}

type TestData struct {
	name   string
	map1   map[int32]*RoleInfo
	slice1 []RoleInfo
}

func NewRole(id int32, name string, level int32) *RoleInfo {
	r := &RoleInfo{
		id:    id,
		name:  name,
		level: level,
	}
	r.setName("") // 防止优化掉未调用过的函数

	return r
}

func NewData(name string) *TestData {
	v := &TestData{
		name: name,
		map1: make(map[int32]*RoleInfo),
	}

	return v
}
