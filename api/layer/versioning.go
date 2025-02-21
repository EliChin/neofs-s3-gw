package layer

import (
	"context"
	"sort"

	"github.com/nspcc-dev/neofs-s3-gw/api/data"
)

const (
	UnversionedObjectVersionID = "null"
)

func (n *layer) ListObjectVersions(ctx context.Context, p *ListObjectVersionsParams) (*ListObjectVersionsInfo, error) {
	var (
		allObjects = make([]*data.ExtendedObjectInfo, 0, p.MaxKeys)
		res        = &ListObjectVersionsInfo{}
	)

	versions, err := n.getAllObjectsVersions(ctx, p.BktInfo, p.Prefix, p.Delimiter)
	if err != nil {
		return nil, err
	}

	sortedNames := make([]string, 0, len(versions))
	for k := range versions {
		sortedNames = append(sortedNames, k)
	}
	sort.Strings(sortedNames)

	for _, name := range sortedNames {
		sortedVersions := versions[name]
		sort.Slice(sortedVersions, func(i, j int) bool {
			return sortedVersions[j].NodeVersion.Timestamp < sortedVersions[i].NodeVersion.Timestamp // sort in reverse order
		})

		for i, version := range sortedVersions {
			version.IsLatest = i == 0
			allObjects = append(allObjects, version)
		}
	}

	for i, obj := range allObjects {
		if obj.ObjectInfo.Name >= p.KeyMarker && obj.ObjectInfo.Version() >= p.VersionIDMarker {
			allObjects = allObjects[i:]
			break
		}
	}

	res.CommonPrefixes, allObjects = triageExtendedObjects(allObjects)

	if len(allObjects) > p.MaxKeys {
		res.IsTruncated = true
		res.NextKeyMarker = allObjects[p.MaxKeys].ObjectInfo.Name
		res.NextVersionIDMarker = allObjects[p.MaxKeys].ObjectInfo.Version()

		allObjects = allObjects[:p.MaxKeys]
		res.KeyMarker = allObjects[p.MaxKeys-1].ObjectInfo.Name
		res.VersionIDMarker = allObjects[p.MaxKeys-1].ObjectInfo.Version()
	}

	objects := make([]*ObjectVersionInfo, len(allObjects))
	for i, obj := range allObjects {
		objects[i] = &ObjectVersionInfo{
			Object:        obj.ObjectInfo,
			IsUnversioned: obj.NodeVersion.IsUnversioned,
			IsLatest:      obj.IsLatest,
		}
	}

	res.Version, res.DeleteMarker = triageVersions(objects)
	return res, nil
}

func triageVersions(objVersions []*ObjectVersionInfo) ([]*ObjectVersionInfo, []*ObjectVersionInfo) {
	if len(objVersions) == 0 {
		return nil, nil
	}

	var resVersion []*ObjectVersionInfo
	var resDelMarkVersions []*ObjectVersionInfo

	for _, version := range objVersions {
		if version.Object.IsDeleteMarker {
			resDelMarkVersions = append(resDelMarkVersions, version)
		} else {
			resVersion = append(resVersion, version)
		}
	}

	return resVersion, resDelMarkVersions
}
