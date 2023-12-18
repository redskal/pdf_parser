/*
 * Added by @sam_phisher to facilitate processing PDFs in-memory
 *
 * We should now be able to parse PDF data as byte arrays.
 * Also fixed some wonky code, but it still needs major refactoring.
 */
package pdf_parser

import (
	"bytes"
	"io"
	"log"
	"regexp"
	"strconv"
)

// Parse pdf file metadata
func ParsePdfInMemory(fileBytes []byte) (*PdfInfo, error) {
	pdfInfo := PdfInfo{}

	version, err := readPdfInfoVersionInMem(fileBytes)
	if err != nil {
		log.Println(err)
		return &pdfInfo, err
	}

	pdfInfo.PdfVersion = version

	pdfInfo.PagesCount = countPagesInMem(fileBytes)

	err = readXrefOffsetInMem(fileBytes, &pdfInfo)
	if err != nil {
		log.Println(err)
		return &pdfInfo, err
	}

	getTrailerSectionInMem(fileBytes, &pdfInfo)

	// original xref
	parsedXref, trailerSection, err := readXrefBlockInMem(fileBytes, pdfInfo.OriginalXrefOffset, true)
	if err != nil {
		log.Println(err)
		return &pdfInfo, err
	}
	pdfInfo.XrefTable = append(pdfInfo.XrefTable, parsedXref)
	pdfInfo.AdditionalTrailerSection = append(pdfInfo.AdditionalTrailerSection, trailerSection)

	readAllXrefSectionsInMem(fileBytes, &pdfInfo, pdfInfo.OriginalTrailerSection.Prev)

	if trailerSection != nil {
		readAllXrefSectionsInMem(fileBytes, &pdfInfo, trailerSection.Prev)
	}

	root := findRootObjectInMem(&pdfInfo, fileBytes)
	if root == nil {
		err = cannotFindRootObject
		log.Println(err)
		return &pdfInfo, nil
	}
	pdfInfo.Root = *root

	info := searchInfoSectionInMem(&pdfInfo, fileBytes)
	if info == nil {
		err = cannotFindInfoObject
		log.Println(err)
		return &pdfInfo, nil
	}
	pdfInfo.Info = *info

	meta, err := findMetadataObjectInMem(&pdfInfo, fileBytes)
	log.Println(err)
	if meta != nil {
		pdfInfo.Metadata = *meta
	}

	return &pdfInfo, nil
}

func readPdfInfoVersionInMem(fileBytes []byte) (string, error) {
	buffer := make([]byte, 15)

	byteReader := bytes.NewReader(fileBytes)
	bytesread, err := byteReader.ReadAt(buffer, 0)
	if err != nil {
		return "", err
	}

	dst, n := binToHex(bytesread, &buffer)
	pdfVersion, err := getPdfVersion((*dst)[:n])
	if err != nil {
		return "", err
	}

	return pdfVersion, nil
}

func countPagesInMem(fileBytes []byte) int {
	buffer := make([]byte, BufferSize300)
	reg := regexp.MustCompile(`(?m)\/Type( )?\/Page([^s])`)
	var (
		offset int64 = 0
		count  int   = 0
	)

	byteReader := bytes.NewReader(fileBytes)

	for {
		bytesRead, err := byteReader.ReadAt(buffer, offset)
		chunk := (buffer)[:bytesRead]

		if err != nil {
			if err == io.EOF {
				resp := reg.FindAllSubmatch(chunk, -1)
				if resp != nil {
					count += len(resp)
				}
				break
			}
		}

		resp := reg.FindAllSubmatch(chunk, -1)
		if resp != nil {
			count += len(resp)
		}
		offset += BufferSize300
	}
	return count
}

func readXrefOffsetInMem(fileBytes []byte, pdfInfo *PdfInfo) error {
	buffer := make([]byte, BufferSize)
	var startXrefOffset = ""

	byteReader := bytes.NewReader(fileBytes)
	bytesRead, err := byteReader.ReadAt(buffer, int64(len(fileBytes))-BufferSize)
	if err != nil {
		return err
	}

	hexBytes, hexBytesWritten := binToHex(bytesRead, &buffer)

	r1 := "(737461727478726566)(0a|0d0a|0d)([a-fa-f0-9]+)(0d0a)(2525)(454f46)"
	r2 := "(737461727478726566)(0a|0d0a|0d)([a-fa-f0-9]+)(0d)(2525)(454f46)"
	r3 := "(737461727478726566)(0a|0d0a|0d)([a-fa-f0-9]+)(0a)(2525)(454f46)"

	if r1Resp := parseRegex(r1, (*hexBytes)[:hexBytesWritten]); r1Resp != nil {
		startXrefOffset, err = decodeHexAsString((*hexBytes)[:hexBytesWritten][r1Resp[0][6]:r1Resp[0][8]])
	} else if r2Resp := parseRegex(r2, (*hexBytes)[:hexBytesWritten]); r2Resp != nil {
		startXrefOffset, err = decodeHexAsString((*hexBytes)[:hexBytesWritten][r2Resp[0][5]:r2Resp[0][7]])
	} else if r3Resp := parseRegex(r3, (*hexBytes)[:hexBytesWritten]); r3Resp != nil {
		startXrefOffset, err = decodeHexAsString((*hexBytes)[:hexBytesWritten][r3Resp[0][5]:r3Resp[0][7]])
	} else {
		return cannotReadXrefOffset
	}

	if err != nil {
		return err
	}

	intVal, err := strconv.ParseInt(startXrefOffset, 10, 64)
	if err != nil {
		return cannotReadXrefOffset
	}

	pdfInfo.OriginalXrefOffset = intVal
	return nil
}

func getTrailerSectionInMem(fileBytes []byte, pdfInfo *PdfInfo) {
	trailer := getFileTrailerInMem(fileBytes, int64(len(fileBytes))-BufferSize300)
	pdfInfo.OriginalTrailerSection = *trailer
}

func getFileTrailerInMem(fileBytes []byte, sectionStart int64) *TrailerSection {
	buffer := make([]byte, BufferSize300*2)
	byteReader := bytes.NewReader(fileBytes)
	bytesRead, err := byteReader.ReadAt(buffer, sectionStart)
	if err != nil {
		if err == io.EOF {
			// continue
		}
	} else {
		log.Println(err)
	}
	hexBytes, hexBytesWritten := binToHex(bytesRead, &buffer)
	slice := (*hexBytes)[:hexBytesWritten]
	trailer, err := parseTrailerBlock(&slice)
	log.Println(err)
	return &trailer
}

func readXrefBlockInMem(fileBytes []byte, xrefOffset int64, trailerRead bool) (*XrefTable, *TrailerSection, error) {
	if xrefOffset == -1 || xrefOffset == 0 {
		return nil, nil, cannotParseXrefOffset
	}

	var bSize int64 = 100
	buffer := make([]byte, bSize)

	offset := xrefOffset
	var xrefBlock [][]byte
	var additionalTrailer TrailerSection

	byteReader := bytes.NewReader(fileBytes)

	for {
		bytesRead, err := byteReader.ReadAt(buffer, offset)
		if err != nil {
			if err == io.EOF {
				hexBytes, hexBytesWritten := binToHex(bytesRead, &buffer)
				xrefBlock = append(xrefBlock, (*hexBytes)[:hexBytesWritten])

				break
			}
			return nil, nil, err
		}

		hexBytes, hexBytesWritten := binToHex(bytesRead, &buffer)
		xrefBlock = append(xrefBlock, (*hexBytes)[:hexBytesWritten])

		if trailerBlockFound(*hexBytes) || checkXrefTrailer(xrefBlock) {
			if trailerRead {
				additionalTrailer = *getFileTrailerInMem(fileBytes, offset-bSize)
			}
			break
		}
		offset += bSize
	}

	// read Xref
	var remaining []byte
	var xrefSectionsString []string
	var trailerCounter = 0
	var trailerStartBlock []byte
	for c, el := range xrefBlock {
		if trailerBlockFound(el) {
			var preTrailer []byte
			preTrailer, trailerStartBlock = getPreTrailerData(el)
			err := readXrefBlockSection(&remaining, &preTrailer, &xrefSectionsString)
			if err != nil {
				return nil, nil, err
			}
			trailerCounter = c
			break
		} else {
			err := readXrefBlockSection(&remaining, &el, &xrefSectionsString)
			if err != nil {
				return nil, nil, err
			}
		}
	}

	var xrefBig []byte
	for i := trailerCounter; i < len(xrefBlock); i++ {
		if i == trailerCounter {
			xrefBig = append(xrefBig, trailerStartBlock...)
			continue
		}
		xrefBig = append(xrefBig, xrefBlock[i]...)
	}

	parsedXref, err := parseXrefSection(&xrefSectionsString)
	if err != nil {
		return nil, nil, err
	}

	parsedXref.SectionStart = xrefOffset
	return parsedXref, &additionalTrailer, nil
}

func readAllXrefSectionsInMem(fileBytes []byte, pdfInfo *PdfInfo, prevOffset int64) {
	if prevOffset != 0 {
		additionalXref, trailer, err := readXrefBlockInMem(fileBytes, prevOffset, true)
		log.Println(err)
		pdfInfo.XrefTable = append(pdfInfo.XrefTable, additionalXref)
		pdfInfo.AdditionalTrailerSection = append(pdfInfo.AdditionalTrailerSection, trailer)

		// we <3 recursion =P
		if trailer != nil {
			readAllXrefSectionsInMem(fileBytes, pdfInfo, trailer.Prev)
		}
	}
}

func findRootObjectInMem(pdfInfo *PdfInfo, fileBytes []byte) *RootObject {
	for _, el := range pdfInfo.AdditionalTrailerSection {
		if el.Root.ObjectNumber != 0 {
			obj, err := readXrefObjectContentInMem(el.Root.ObjectNumber, pdfInfo, fileBytes)
			if err != nil {
				log.Println(err)
			}

			parsedObj, err := parseObjectContent(obj)
			if err != nil {
				log.Println(err)
			}
			return parseRootObject(parsedObj)
		}
	}
	return nil
}

func readXrefObjectContentInMem(objectNumber int, pdfInfo *PdfInfo, fileBytes []byte) ([]byte, error) {
	var offset int64 = 0

	for _, xrefTable := range pdfInfo.XrefTable {
		if xrefTable == nil {
			// TODO fix for object xref
			return nil, invalidXrefTableStructure
		}
		if obj, ok := xrefTable.Objects[objectNumber]; ok {
			offset = int64(obj.ObjectNumber)
		}
	}

	if offset == 0 {
		return nil, cannotFindObjectById
	}

	var (
		bSize      int64 = 100
		data       []byte
		blocksRead = 0
	)

	byteReader := bytes.NewReader(fileBytes)

	for buffer := make([]byte, bSize); ; blocksRead++ {
		bytesRead, err := byteReader.ReadAt(buffer, offset)
		if err != nil {
			if err == io.EOF {
				return nil, cannotParseObject
			}
			return nil, err
		}

		x := (buffer)[:bytesRead]
		if blocksRead == 0 && !bytes.Contains(x, []byte("obj")) {
			return nil, cannotParseObject
		}

		data = append(data, x...)
		offset += bSize

		if bytes.Contains(x, []byte("endobj")) {
			break
		}
	}

	return data, nil
}

func searchInfoSectionInMem(pdfInfo *PdfInfo, fileBytes []byte) *InfoObject {
	for _, el := range pdfInfo.AdditionalTrailerSection {
		if el.Info.ObjectNumber != 0 {
			obj, err := readXrefObjectContentInMem(el.Info.ObjectNumber, pdfInfo, fileBytes)
			if err != nil {
				log.Println(err)
			}

			parsedObj, err := parseObjectContent(obj)
			if err != nil {
				log.Println(err)
			}
			return parseInfoObject(parsedObj)
		}
	}
	return nil
}

func findMetadataObjectInMem(pdfInfo *PdfInfo, fileBytes []byte) (*Metadata, error) {
	if pdfInfo.Root.Metadata != nil && pdfInfo.Root.Metadata.ObjectNumber != 0 {
		obj, err := readXrefObjectContentInMem(pdfInfo.Root.Metadata.ObjectNumber, pdfInfo, fileBytes)
		log.Println(err)
		return parseMetadataContent(obj)
	}

	return nil, cannotFindObjectById
}
