package annotations

import "context"

type RW interface {
	Read(ctx context.Context, contentUUID string) ([]*Annotation, bool, error)
}

type annotationsRW struct {
	endpoint string
}

func NewRW(endpoint string) RW {
	return &annotationsRW{endpoint}
}

func (rw *annotationsRW) Read(ctx context.Context, contentUUID string) ([]*Annotation, bool, error) {
	return []*Annotation{}, false, nil
}
