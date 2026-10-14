package object

import "github.com/brunoga/deep"

type AccessMode string

const (
	AccessModeRead      AccessMode = "ReadOnly"
	AccessModeWrite     AccessMode = "WriteOnly"
	AccessModeReadWrite AccessMode = "ReadWrite"
)

type BucketAccess struct {
	Name      string
	StoreName string
}

type BucketAccessMode struct {
	BucketName string
	AccessMode AccessMode
}

// S3BucketAccessSecret contains the credentials returned when bucket access is granted.
// It is intentionally excluded from Trident persistence and must never be logged.
type S3BucketAccessSecret struct {
	AccessKeyID     string
	SecretAccessKey string
}

func (a *BucketAccess) SmartCopy() interface{} {
	return deep.MustCopy(a)
}

func (a *BucketAccess) GetStoreName() string {
	return a.StoreName
}

func (a *BucketAccess) GetObjectStoreID() string {
	return a.StoreName
}
