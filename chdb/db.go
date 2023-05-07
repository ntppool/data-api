package chdb

type ClickHouse struct {
}

func New(dbConfigPath string) (*ClickHouse, error) {
	return &ClickHouse{}, nil
}
