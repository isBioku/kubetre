package airlock

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/service"
)

// AzureStore keeps airlock data in two Azure Storage accounts. It authenticates with the
// environment's workload identity; the accounts have shared keys disabled, so links are user
// delegation SAS signed with Entra ID.
type AzureStore struct {
	names   map[Account]string
	clients map[Account]*service.Client
}

// NewAzureStore connects to the external and internal accounts by name.
func NewAzureStore(external, internal string) (*AzureStore, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}
	return newAzureStore(cred, map[Account]string{External: external, Internal: internal})
}

func newAzureStore(cred azcore.TokenCredential, names map[Account]string) (*AzureStore, error) {
	s := &AzureStore{names: names, clients: map[Account]*service.Client{}}
	for acct, name := range names {
		c, err := service.NewClient(fmt.Sprintf("https://%s.blob.core.windows.net/", name), cred, nil)
		if err != nil {
			return nil, err
		}
		s.clients[acct] = c
	}
	return s, nil
}

func (s *AzureStore) container(acct Account, name string) *container.Client {
	return s.clients[acct].NewContainerClient(name)
}

func (s *AzureStore) EnsureContainer(ctx context.Context, acct Account, name string) error {
	_, err := s.container(acct, name).Create(ctx, nil)
	if bloberror.HasCode(err, bloberror.ContainerAlreadyExists) {
		return nil
	}
	return err
}

func (s *AzureStore) DeleteContainer(ctx context.Context, acct Account, name string) error {
	_, err := s.container(acct, name).Delete(ctx, nil)
	if bloberror.HasCode(err, bloberror.ContainerNotFound, bloberror.ContainerBeingDeleted) {
		return nil
	}
	return err
}

func (s *AzureStore) ListFiles(ctx context.Context, acct Account, name string) ([]File, error) {
	out := []File{}
	pager := s.container(acct, name).NewListBlobsFlatPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if bloberror.HasCode(err, bloberror.ContainerNotFound) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		for _, b := range page.Segment.BlobItems {
			size := int64(0)
			if b.Properties != nil && b.Properties.ContentLength != nil {
				size = *b.Properties.ContentLength
			}
			out = append(out, File{Name: *b.Name, Size: size})
		}
	}
	return out, nil
}

// Copy streams each blob through this process. Server-side copy cannot be used: the internal
// account has no public endpoint for Azure to read from.
func (s *AzureStore) Copy(ctx context.Context, from Account, fromName string, to Account, toName string) error {
	files, err := s.ListFiles(ctx, from, fromName)
	if err != nil {
		return err
	}
	if err := s.EnsureContainer(ctx, to, toName); err != nil {
		return err
	}
	src, dst := s.container(from, fromName), s.container(to, toName)
	for _, f := range files {
		body, err := src.NewBlobClient(f.Name).DownloadStream(ctx, nil)
		if err != nil {
			return fmt.Errorf("download %s: %w", f.Name, err)
		}
		_, err = dst.NewBlockBlobClient(f.Name).UploadStream(ctx, body.Body, nil)
		_ = body.Body.Close()
		if err != nil {
			return fmt.Errorf("upload %s: %w", f.Name, err)
		}
	}
	return nil
}

func (s *AzureStore) ScanVerdicts(ctx context.Context, acct Account, name string) (map[string]string, error) {
	files, err := s.ListFiles(ctx, acct, name)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, f := range files {
		tags, err := s.container(acct, name).NewBlobClient(f.Name).GetTags(ctx, nil)
		if err != nil {
			return nil, err
		}
		for _, t := range tags.BlobTagSet {
			if t.Key != nil && *t.Key == ScanTag && t.Value != nil {
				out[f.Name] = *t.Value
			}
		}
	}
	return out, nil
}

// ContainerURL signs a container link with a user delegation key valid for the same window.
// The start is backdated 15 minutes for clock skew, as AzureTRE does.
func (s *AzureStore) ContainerURL(ctx context.Context, acct Account, name string, perm Permissions, valid time.Duration) (string, error) {
	start := time.Now().UTC().Add(-15 * time.Minute)
	expiry := time.Now().UTC().Add(valid)
	info := service.KeyInfo{Start: to.Ptr(start.Format(sas.TimeFormat)), Expiry: to.Ptr(expiry.Format(sas.TimeFormat))}
	udc, err := s.clients[acct].GetUserDelegationCredential(ctx, info, nil)
	if err != nil {
		return "", fmt.Errorf("user delegation key: %w", err)
	}
	perms := sas.ContainerPermissions{Read: perm.Read, Write: perm.Write, Delete: perm.Delete, List: perm.List,
		Create: perm.Write, Add: perm.Write}
	q, err := sas.BlobSignatureValues{
		Protocol: sas.ProtocolHTTPS, StartTime: start, ExpiryTime: expiry,
		Permissions: perms.String(), ContainerName: name,
	}.SignWithUserDelegation(udc)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://%s.blob.core.windows.net/%s?%s", s.names[acct], name, q.Encode()), nil
}

var _ Store = (*AzureStore)(nil)
var _ Store = (*MemoryStore)(nil)
