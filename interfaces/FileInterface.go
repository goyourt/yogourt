package interfaces

type FileInterface interface {
	BaseInterface
	GetName() string
	SetName(name string)
	GetPath() string
	SetPath(path string)
	GetExtension() string
	SetExtension(extension string)
	GetContent() string
	SetContent(content string)
}

type File struct {
	Base
	Name      string `gorm:"not null" json:"Name"`
	Path      string `gorm:"not null" json:"-"`
	Extension string `json:"Extension"`
	Content   string `gorm:"-" json:"Content"`
}

func (f *File) GetName() string { return f.Name }

func (f *File) SetName(name string) { f.Name = name }

func (f *File) GetPath() string { return f.Path }

func (f *File) SetPath(path string) { f.Path = path }

func (f *File) GetExtension() string { return f.Extension }

func (f *File) SetExtension(extension string) { f.Extension = extension }

func (f *File) GetContent() string { return f.Content }

func (f *File) SetContent(content string) { f.Content = content }
