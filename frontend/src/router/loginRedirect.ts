/** Resolve only local return URLs, never another login/register page. */
export function resolveLoginRedirect(
  redirect: unknown,
  fallback: string,
  inheritedHash = ''
): string {
  if (typeof redirect !== 'string' || !redirect.startsWith('/') || redirect.startsWith('//')) {
    return fallback
  }

  try {
    const url = new URL(redirect, 'https://app.invalid')
    const decodedPath = decodeURIComponent(url.pathname)
    // Reject backslashes/control characters and encoded protocol-relative paths.
    if (
      url.origin !== 'https://app.invalid' ||
      [...redirect, ...decodedPath].some((char) => char === '\\' || char.charCodeAt(0) <= 32) ||
      decodedPath.startsWith('//') ||
      /^\/(login|register)\/*$/i.test(decodedPath)
    ) {
      return fallback
    }
    // A server redirect cannot read the fragment; browsers inherit it on /login.
    return url.pathname + url.search + (url.hash || inheritedHash)
  } catch {
    return fallback
  }
}
