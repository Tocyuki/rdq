/**
 * Error-code constants that mirror the `errCode*` enum in
 * `/Users/tocyuki/ghq/github.com/Tocyuki/rdq/internal/server/errors.go`.
 * Keep the values in sync whenever a new code is added server-side.
 */
export const ErrorCode = {
  BadRequest: 'bad_request',
  NotFound: 'not_found',
  OriginDenied: 'origin_denied',
  Unauthorized: 'unauthorized',
  Timeout: 'timeout',
  AWSError: 'aws_error',
  Internal: 'internal',
  ReadOnly: 'read_only',
} as const

export type ErrorCodeValue = (typeof ErrorCode)[keyof typeof ErrorCode]
