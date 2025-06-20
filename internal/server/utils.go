package server

import "strconv"

// convertValue converts []byte values to appropriate types for JSON marshaling
func convertValue(val interface{}) interface{} {
	if val == nil {
		return nil
	}
	
	// Handle []byte values that might be numeric
	if bytes, ok := val.([]byte); ok {
		str := string(bytes)
		
		// Try to parse as float first (handles both integers and decimals)
		if f, err := strconv.ParseFloat(str, 64); err == nil {
			// If it's a whole number, return as int64 for smaller JSON representation
			if f == float64(int64(f)) {
				return int64(f)
			}
			return f
		}
		
		// If not numeric, return as string
		return str
	}
	
	return val
}