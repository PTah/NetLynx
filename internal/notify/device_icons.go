package notify

import (
	"embed"
	"path"
	"strings"
)

// White-layer PNG для писем (светлый фон клиента почты).
// В UI — другой набор: web/public/device-icons (стандартный пак).
//
//go:embed assets/device-icons/*.png
var deviceIconFS embed.FS

// DeviceIconCID возвращает Content-ID без < > (по имени файла иконки).
func DeviceIconCID(category string) string {
	return "netlynx-icon-" + iconFileStem(category)
}

// DeviceIconPNG — PNG white-layer для категории; неизвестные → other.
func DeviceIconPNG(category string) []byte {
	stem := iconFileStem(category)
	data, err := deviceIconFS.ReadFile(path.Join("assets/device-icons", stem+".png"))
	if err != nil {
		data, err = deviceIconFS.ReadFile("assets/device-icons/other.png")
		if err != nil {
			return nil
		}
	}
	return data
}

// iconFileStem — имя файла без .png (совпадает с web/public/device-icons).
func iconFileStem(raw string) string {
	c := strings.ToLower(strings.TrimSpace(raw))
	switch c {
	case "switch", "router", "ap", "server", "computer", "phone", "mfu", "camera",
		"other", "tv", "rack", "industrial":
		return c
	case "ilo", "ipmi":
		return "ilo-idrac-ipmi"
	default:
		return "other"
	}
}

// CollectDeviceIconInline — уникальные inline-вложения для карточек письма.
func CollectDeviceIconInline(cards []EmailDeviceCard) []InlineImage {
	seen := make(map[string]struct{}, len(cards))
	out := make([]InlineImage, 0, 4)
	for _, c := range cards {
		stem := iconFileStem(c.Category)
		if _, ok := seen[stem]; ok {
			continue
		}
		seen[stem] = struct{}{}
		data := DeviceIconPNG(stem)
		if len(data) == 0 {
			continue
		}
		out = append(out, InlineImage{
			CID:         DeviceIconCID(stem),
			ContentType: "image/png",
			Data:        data,
		})
	}
	return out
}
