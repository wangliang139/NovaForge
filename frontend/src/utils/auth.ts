/**
 * Token 管理工具
 * 用于前端 JWT token 的存储、读取和清除
 */

const TOKEN_KEY = 'llt_access_token';

/**
 * 获取存储的 access token
 */
export function getAccessToken(): string | null {
  try {
    return localStorage.getItem(TOKEN_KEY);
  } catch (error) {
    console.error('Failed to get access token:', error);
    return null;
  }
}

/**
 * 存储 access token
 */
export function setAccessToken(token: string): void {
  try {
    localStorage.setItem(TOKEN_KEY, token);
  } catch (error) {
    console.error('Failed to set access token:', error);
  }
}

/**
 * 清除存储的 access token
 */
export function clearAccessToken(): void {
  try {
    localStorage.removeItem(TOKEN_KEY);
  } catch (error) {
    console.error('Failed to clear access token:', error);
  }
}
