# pdf_parser by flotzilla - updated by redskal/@sam_phisher

I was looking for a simple PDF parser library and found [this one](https://github.com/flotzilla/pdf_parser), but it
only worked from files on-disk. My use case required me to process data in-memory, so I've adapted it.

It's pretty much a copy-pasta of the relevant functions which were then modified slightly. Hopefully I'll get around
to reimplementing this properly from scratch, or with some major refactoring. For now, it serves the purpose for my PoC.

Usage should be something like:

```go
import "github.com/redskal/pdf_parser"

pdf, err := pdf_parser.ParsePdfInMem(byteArrayOfPdfFile)

pdf.GetTitle()
pdf.GetAuthor()
pdf.GetCreator()
pdf.GetISBN()
pdf.GetPublishers() []string
pdf.GetLanguages() []string
pdf.GetDescription()
pdf.GetPagesCount()
```

Original README is below.

---

Pdf metadata parser
====
Go library for parsing pdf metadata information 
 
### License
MIT 

### Usage
```go
import "github.com/flotzilla/pdf_parser"

// parse file
pdf, errors := pdf_parser.ParsePdf("filepath/file.pdf")

// main functions
pdf.GetTitle()
pdf.GetAuthor()
pdf.GetCreator()
pdf.GetISBN()
pdf.GetPublishers() []string
pdf.GetLanguages() []string
pdf.GetDescription()
pdf.GetPagesCount()
```

Using with custom `github.com/sirupsen/logrus` logger

```go
import "github.com/flotzilla/pdf_parser"

l := logger.New()
l.SetOutput(os.Stdout)
lg.SetFormatter(&logger.JSONFormatter{})

SetLogger(lg)
file, _ := filepath.Abs("filepath/file.pdf")
pdf, err := ParsePdf(file)

```
