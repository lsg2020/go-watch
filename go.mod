module github.com/lsg2020/go-watch

go 1.17

require (
	github.com/agiledragon/gomonkey v2.0.2+incompatible
	github.com/yuin/gopher-lua v0.0.0-20210529063254-f4c35e4016d9
	github.com/zeebo/goof v0.0.0-20190312211016-1ee209ef0510
)

replace github.com/zeebo/goof v0.0.0-20190312211016-1ee209ef0510 => github.com/lsg2020/goof v0.0.0-20220501034542-a04e25b84229

require github.com/zeebo/errs v1.1.1 // indirect
