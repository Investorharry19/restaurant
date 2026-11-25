package httpapi

import "time"

func restaurantBusinessDate(now time.Time, timezone string) (string, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return "", err
	}
	return now.In(location).Format("2006-01-02"), nil
}
