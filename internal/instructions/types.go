package instructions

const (
	FileName        = "MEWCODE.md"
	MaxIncludeDepth = 5
)

type Block struct {
	Source   string
	Priority int
	Content  string
}

type Result struct {
	Blocks   []Block
	Warnings []string
}
