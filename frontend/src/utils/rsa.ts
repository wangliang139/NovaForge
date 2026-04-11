import { JSEncrypt } from 'jsencrypt';
import { EncryptKey } from '../../config/encrypt';

/**
 * 使用 config/routes 中的公钥对密码做 RSA（PKCS#1 v1.5）加密，返回 Base64 密文。
 */
export function encrypt(text: string): string {
  const enc = new JSEncrypt({ default_key_size: '2048' });
  enc.setPublicKey(EncryptKey.trim());
  const out = enc.encrypt(text);
  if (!out) {
    throw new Error('RSA 加密失败，请检查公钥');
  }
  return out;
}
