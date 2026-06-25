package mywant

// MemoReader provides read-only access to the memo store.
type MemoReader interface {
	// GetCategory returns all values stored under the given memo category key (e.g. "cities").
	GetCategory(key string) []string
}

var globalMemoReader MemoReader

func GetGlobalMemoReader() MemoReader { return globalMemoReader }
func SetGlobalMemoReader(r MemoReader) { globalMemoReader = r }
