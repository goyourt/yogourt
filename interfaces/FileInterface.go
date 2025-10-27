package interfaces

type FileInterface interface {
	GetUuid() string
	GetName() string
	GetExtension() string
	GetContent() string
}
