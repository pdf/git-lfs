package lfs

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/github/git-lfs/api"
	"github.com/github/git-lfs/config"
	"github.com/github/git-lfs/errutil"
	"github.com/github/git-lfs/progress"
)

// Uploadable describes a file that can be uploaded.
type Uploadable struct {
	oid      string
	OidPath  string
	Filename string
	size     int64
	object   *api.ObjectResource
}

// NewUploadable builds the Uploadable from the given information.
// "filename" can be empty if a raw object is pushed (see "object-id" flag in push command)/
func NewUploadable(oid, filename string) (*Uploadable, error) {
	localMediaPath, err := LocalMediaPath(oid)
	if err != nil {
		return nil, errutil.Errorf(err, "Error uploading file %s (%s)", filename, oid)
	}

	if len(filename) > 0 {
		if err := ensureFile(filename, localMediaPath); err != nil {
			return nil, err
		}
	}

	fi, err := os.Stat(localMediaPath)
	if err != nil {
		return nil, errutil.Errorf(err, "Error uploading file %s (%s)", filename, oid)
	}

	return &Uploadable{oid: oid, OidPath: localMediaPath, Filename: filename, size: fi.Size()}, nil
}

func (u *Uploadable) Check() (*api.ObjectResource, error) {
	return api.UploadCheck(u.OidPath)
}

func (u *Uploadable) Transfer(cb progress.CopyCallback) error {
	wcb := func(total, read int64, current int) error {
		cb(total, read, current)
		return nil
	}

	path, err := LocalMediaPath(u.object.Oid)
	if err != nil {
		return errutil.Error(err)
	}

	file, err := os.Open(path)
	if err != nil {
		return errutil.Error(err)
	}
	defer file.Close()

	reader := &progress.CallbackReader{
		C:         wcb,
		TotalSize: u.object.Size,
		Reader:    file,
	}

	return api.UploadObject(u.object, reader)
}

func (u *Uploadable) Object() *api.ObjectResource {
	return u.object
}

func (u *Uploadable) Oid() string {
	return u.oid
}

func (u *Uploadable) Size() int64 {
	return u.size
}

func (u *Uploadable) Name() string {
	return u.Filename
}

func (u *Uploadable) SetObject(o *api.ObjectResource) {
	u.object = o
}

// NewUploadQueue builds an UploadQueue, allowing `workers` concurrent uploads.
func NewUploadQueue(files int, size int64, dryRun bool) *TransferQueue {
	q := newTransferQueue(files, size, dryRun)
	q.transferKind = "upload"
	return q
}

// ensureFile makes sure that the cleanPath exists before pushing it.  If it
// does not exist, it attempts to clean it by reading the file at smudgePath.
func ensureFile(smudgePath, cleanPath string) error {
	if _, err := os.Stat(cleanPath); err == nil {
		return nil
	}

	expectedOid := filepath.Base(cleanPath)
	localPath := filepath.Join(config.LocalWorkingDir, smudgePath)
	file, err := os.Open(localPath)
	if err != nil {
		return err
	}

	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return err
	}

	cleaned, err := PointerClean(file, file.Name(), stat.Size(), nil)
	if cleaned != nil {
		cleaned.Teardown()
	}

	if err != nil {
		return err
	}

	if expectedOid != cleaned.Oid {
		return fmt.Errorf("Trying to push %q with OID %s.\nNot found in %s.", smudgePath, expectedOid, filepath.Dir(cleanPath))
	}

	return nil
}
