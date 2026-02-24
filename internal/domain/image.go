package domain

import (
	"sort"
	"strings"
)

// Image type constants matching PHP Image class.
const (
	ImageTypeUnknown      = 0
	ImageTypeClassInside  = 1
	ImageTypeClassOutside = 2
	ImageTypeOperatorLogo = 3
	ImageTypeStation      = 4
	ImageTypeRoute        = 5
	ImageTypeMedia        = 6
	ImageTypeProduct      = 7
	ImageTypeStationMap   = 13
	ImageTypeClassOther   = 16
)

// ImageTuple is a 5-element image representation: [path, width, height, type, imageOrder].
type ImageTuple = []any

// classImageKey indexes class images by operator+class.
type classImageKey struct {
	OperatorID int
	ClassID    int
}

// customImageEntry stores a custom class image with its matching criteria.
// Uses pointers for nullable fields to match PHP NULL semantics.
type customImageEntry struct {
	OperatorID int
	ClassID    int
	Image      ImageTuple
	OfficialID *string // nil = no official_id constraint
	FromID     *int    // nil = no from_id constraint
	ToID       *int    // nil = no to_id constraint
}

// ImageCollection replicates PHP ImageCollection for trip image resolution.
// Storage mirrors PHP's 4 separate nested arrays for custom images:
//
//	customClassImagesFullMatch[op][class][official][from][to]
//	customClassImagesFromTo[op][class][from][to]
//	customClassImagesOfficial[op][class][official]
//	customClassImages[op][class]
type ImageCollection struct {
	classImages map[classImageKey][]ImageTuple

	customFullMatch []customImageEntry
	customFromTo    []customImageEntry
	customOfficial  []customImageEntry
	customClassOnly []customImageEntry

	routeImages map[int][]ImageTuple
}

// NewImageCollection creates an empty ImageCollection.
func NewImageCollection() *ImageCollection {
	return &ImageCollection{
		classImages: make(map[classImageKey][]ImageTuple),
		routeImages: make(map[int][]ImageTuple),
	}
}

// AddClassImage adds a class image (from images_class table).
func (c *ImageCollection) AddClassImage(operatorID, classID int, img ImageTuple) {
	key := classImageKey{operatorID, classID}
	c.classImages[key] = append(c.classImages[key], img)
}

// AddCustomClassImage adds a custom class image, storing it in the appropriate
// priority bucket matching PHP's addCustomClassImage() logic.
// Parameters use pointers to preserve PHP NULL semantics from the DB.
func (c *ImageCollection) AddCustomClassImage(operatorID, classID int, img ImageTuple, officialID *string, fromID, toID *int) {
	entry := customImageEntry{
		OperatorID: operatorID,
		ClassID:    classID,
		Image:      img,
		OfficialID: officialID,
		FromID:     fromID,
		ToID:       toID,
	}

	// PHP truthiness: non-empty string, non-zero int
	hasOfficial := officialID != nil && *officialID != ""
	hasFrom := fromID != nil && *fromID != 0
	hasTo := toID != nil && *toID != 0

	// PHP logic from ImageCollection::addCustomClassImage():
	// if ($officialId && $fromStationId && $toStationId) → fullMatch
	// elseif ($fromStationId && $toStationId) → fromTo
	// elseif ($officialId) → official
	// elseif (!$officialId) → classOnly
	if hasOfficial && hasFrom && hasTo {
		c.customFullMatch = append(c.customFullMatch, entry)
	} else if hasFrom && hasTo {
		c.customFromTo = append(c.customFromTo, entry)
	} else if hasOfficial {
		c.customOfficial = append(c.customOfficial, entry)
	} else if !hasOfficial {
		c.customClassOnly = append(c.customClassOnly, entry)
	}
}

// AddRouteImage adds a route image (from images_route table).
func (c *ImageCollection) AddRouteImage(routeID int, img ImageTuple) {
	c.routeImages[routeID] = append(c.routeImages[routeID], img)
}

// GetTripImages returns the best-matching images for a trip, following PHP's
// findCustomClassImages() priority chain, then route, then class images.
func (c *ImageCollection) GetTripImages(operatorID, classID int, officialID string, fromID, toID, routeID int) []any {
	// Priority 1: Full match (operator+class+official+from+to)
	if imgs := c.findCustomFullMatch(operatorID, classID, officialID, fromID, toID); len(imgs) > 0 {
		return sortAndWrapImages(imgs)
	}

	// Priority 2: From/To match (operator+class+from+to)
	if imgs := c.findCustomFromTo(operatorID, classID, fromID, toID); len(imgs) > 0 {
		return sortAndWrapImages(imgs)
	}

	// Priority 3: Official match (operator+class+official)
	if imgs := c.findCustomOfficial(operatorID, classID, officialID); len(imgs) > 0 {
		return sortAndWrapImages(imgs)
	}

	// Priority 4: Class-only match (operator+class)
	if imgs := c.findCustomClassOnly(operatorID, classID); len(imgs) > 0 {
		return sortAndWrapImages(imgs)
	}

	// Priority 5: Route images
	if routeID > 0 {
		if imgs, ok := c.routeImages[routeID]; ok && len(imgs) > 0 {
			return sortAndWrapImages(imgs)
		}
	}

	// Priority 6: Class images
	key := classImageKey{operatorID, classID}
	if imgs, ok := c.classImages[key]; ok && len(imgs) > 0 {
		return sortAndWrapImages(imgs)
	}

	return []any{}
}

func (c *ImageCollection) findCustomFullMatch(opID, classID int, officialID string, fromID, toID int) []ImageTuple {
	var result []ImageTuple
	for _, e := range c.customFullMatch {
		if e.OperatorID == opID && e.ClassID == classID &&
			e.OfficialID != nil && *e.OfficialID == officialID &&
			e.FromID != nil && *e.FromID == fromID &&
			e.ToID != nil && *e.ToID == toID {
			result = append(result, e.Image)
		}
	}
	return result
}

func (c *ImageCollection) findCustomFromTo(opID, classID int, fromID, toID int) []ImageTuple {
	var result []ImageTuple
	for _, e := range c.customFromTo {
		if e.OperatorID == opID && e.ClassID == classID &&
			e.FromID != nil && *e.FromID == fromID &&
			e.ToID != nil && *e.ToID == toID {
			result = append(result, e.Image)
		}
	}
	return result
}

func (c *ImageCollection) findCustomOfficial(opID, classID int, officialID string) []ImageTuple {
	var result []ImageTuple
	for _, e := range c.customOfficial {
		if e.OperatorID == opID && e.ClassID == classID &&
			e.OfficialID != nil && *e.OfficialID == officialID {
			result = append(result, e.Image)
		}
	}
	return result
}

func (c *ImageCollection) findCustomClassOnly(opID, classID int) []ImageTuple {
	var result []ImageTuple
	for _, e := range c.customClassOnly {
		if e.OperatorID == opID && e.ClassID == classID {
			result = append(result, e.Image)
		}
	}
	return result
}

// sortAndWrapImages sorts images matching PHP's compareImages() logic:
// 1. Sort by type priority
// 2. For TYPE_CLASS_OTHER: sort by imageOrder, then by key
// 3. For same priority: sort by key (strcmp)
func sortAndWrapImages(imgs []ImageTuple) []any {
	sort.SliceStable(imgs, func(i, j int) bool {
		return compareImages(imgs[i], imgs[j]) < 0
	})
	result := make([]any, len(imgs))
	for i, img := range imgs {
		result[i] = img
	}
	return result
}

// compareImages replicates PHP ImageCollection::compareImages().
func compareImages(a, b ImageTuple) int {
	ta := imageType(a)
	tb := imageType(b)

	// Both are TYPE_CLASS_OTHER: sort by imageOrder, then by key
	if ta == ImageTypeClassOther && tb == ImageTypeClassOther {
		ia := imageOrder(a)
		ib := imageOrder(b)
		if ia != ib {
			return ia - ib
		}
		return strings.Compare(imageKey(a), imageKey(b))
	}

	pa := imagePriority(ta)
	pb := imagePriority(tb)
	if pa != pb {
		return pa - pb
	}

	return strings.Compare(imageKey(a), imageKey(b))
}

func imageType(img ImageTuple) int {
	if len(img) < 4 {
		return 0
	}
	t, ok := img[3].(int)
	if !ok {
		return 0
	}
	return t
}

func imageOrder(img ImageTuple) int {
	if len(img) < 5 {
		return 0
	}
	o, ok := img[4].(int)
	if !ok {
		return 0
	}
	return o
}

func imageKey(img ImageTuple) string {
	if len(img) < 1 {
		return ""
	}
	s, ok := img[0].(string)
	if !ok {
		return ""
	}
	return s
}

func imagePriority(t int) int {
	switch t {
	case ImageTypeClassInside:
		return 100
	case ImageTypeClassOutside:
		return 200
	case ImageTypeClassOther:
		return 10000
	default:
		return 9000 + t
	}
}
