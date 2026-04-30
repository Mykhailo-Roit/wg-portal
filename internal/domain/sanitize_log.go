package domain

import "log/slog"

func LogSanitizationChange(
	providerType string,
	providerName string,
	field string,
	raw string,
	sanitizeFn func() string,
	dest *string,
) {
	sanitized := sanitizeFn()
	if sanitized != raw {
		if sanitized == "" {
			slog.Warn("sanitization cleared field value from external provider",
				"provider_type", providerType,
				"provider", providerName,
				"field", field,
			)
		} else {
			slog.Warn("sanitization modified field value from external provider",
				"provider_type", providerType,
				"provider", providerName,
				"field", field,
			)
		}
	}
	*dest = sanitized
}
