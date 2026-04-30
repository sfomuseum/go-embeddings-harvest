package models

var lookup = map[string]int{
	"apple/mobileclip_s0":                 512,
	"apple/mobileclip_s1":                 512,
	"apple/mobileclip_s2":                 512,
	"google/siglip-base-patch16-224":      1024,
	"openclip/ViT-g-14#laion2b_s34b_b88k": 1024,
}

func DeriveDimensionsFromModel(model string) (int, bool) {
	d, exists := lookup[model]
	return d, exists
}
