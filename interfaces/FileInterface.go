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
	Name      *string `json:"name"`
	Path      *string `json:"-"`
	Extension *string `json:"-"`
	Content   *string `gorm:"-" json:"-"`
}

func (f *File) GetName() string {
	if nil == f.Name {
		return ""
	}
	return *f.Name
}

func (f *File) SetName(name string) {
	if nil == f.Name {
		f.Name = new(string)
	}
	*f.Name = name
}

func (f *File) GetPath() string {
	if nil == f.Path {
		return ""
	}
	return *f.Path
}

func (f *File) SetPath(path string) {
	if nil == f.Path {
		f.Path = new(string)
	}
	*f.Path = path
}

func (f *File) GetExtension() string {
	if nil == f.Extension {
		return ""
	}
	return *f.Extension
}

func (f *File) SetExtension(extension string) {
	if nil == f.Extension {
		f.Extension = new(string)
	}
	*f.Extension = extension
}

func (f *File) GetContent() string {
	if nil == f.Content {
		return ""
	}
	return *f.Content
}

func (f *File) SetContent(content string) {
	if nil == f.Content {
		f.Content = new(string)
	}
	*f.Content = content
}
