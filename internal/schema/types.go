package schema

type Table struct {
	Name        string
	Schema      string
	Columns     []Column
	PrimaryKeys []string
	ForeignKeys []ForeignKey
	Indexes     []Index
	RowCount    int64
}

type Column struct {
	Name         string
	DataType     string
	IsNullable   bool
	DefaultValue *string
	MaxLength    *int
	Position     int
}

type ForeignKey struct {
	Name             string
	ColumnName       string
	ReferencedTable  string
	ReferencedColumn string
	ReferencedSchema string
	OnDelete         string
	OnUpdate         string
}

type Index struct {
	Name      string
	TableName string
	Columns   []string
	IsUnique  bool
	IsPrimary bool
	IndexType string
}

type Sequence struct {
	Name        string
	Schema      string
	LastValue   int64
	StartValue  int64
	IncrementBy int64
	MinValue    *int64
	MaxValue    *int64
	CacheValue  int64
	IsCycle     bool
}

type View struct {
	Name       string
	Schema     string
	Definition string
}

type Function struct {
	Name       string
	Schema     string
	Definition string
	Language   string
	ReturnType string
}
