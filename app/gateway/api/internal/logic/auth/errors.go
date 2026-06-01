package auth

// This file previously contained sanitizeAuthRPCError and method-level
// wrappers. All call sites now use errorx.SanitizeGRPCError directly.
