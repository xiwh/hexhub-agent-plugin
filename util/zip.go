package util

import (
	"archive/zip"
	"github.com/wonderivan/logger"
	"io"
	"os"
	"path"
	"path/filepath"
)

func Unzip(src string, dest string, fileMode os.FileMode) error {
	reader, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func(reader *zip.ReadCloser) {
		err := reader.Close()
		if err != nil {
			logger.Error(err)
		}
	}(reader)

	for _, file := range reader.File {
		filePath := path.Join(dest, file.Name)
		if file.FileInfo().IsDir() {
			err := os.MkdirAll(filePath, fileMode)
			if err != nil {
				logger.Error(err)
			}
		} else {
			if err = os.MkdirAll(filepath.Dir(filePath), fileMode); err != nil {
				return err
			}
			inFile, err := file.Open()
			if err != nil {
				return err
			}
			defer func() {
				err := inFile.Close()
				if err != nil {
					logger.Error(err)
				}
			}()

			outFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
			if err != nil {
				return err
			}
			defer func() {
				err := outFile.Close()
				if err != nil {
					logger.Error(err)
				}
			}()

			_, err = io.Copy(outFile, inFile)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
