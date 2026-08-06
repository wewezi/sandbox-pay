import { redact } from './redact';

describe('redact', () => {
  it('redacts sensitive keys in objects', () => {
    const input = { username: 'admin', password: 'secret123', token: 'abc' };
    const result = redact(input);

    expect(result.username).toBe('admin');
    expect(result.password).toBe('***REDACTED***');
    expect(result.token).toBe('***REDACTED***');
  });

  it('is case-insensitive for key matching', () => {
    const input = { Password: 'x', APP_SECRET: 'y', Authorization: 'z' };
    const result = redact(input);

    expect(result.Password).toBe('***REDACTED***');
    expect(result.APP_SECRET).toBe('***REDACTED***');
    expect(result.Authorization).toBe('***REDACTED***');
  });

  it('redacts nested objects', () => {
    const input = { user: { name: 'admin', password: 'secret' } };
    const result = redact(input);

    expect(result.user.name).toBe('admin');
    expect(result.user.password).toBe('***REDACTED***');
  });

  it('redacts arrays of objects', () => {
    const input = [{ password: 'a' }, { token: 'b' }];
    const result = redact(input);

    expect(result[0].password).toBe('***REDACTED***');
    expect(result[1].token).toBe('***REDACTED***');
  });

  it('redacts bearer tokens in strings', () => {
    const input = 'Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U';
    const result = redact(input);

    expect(result).toContain('***REDACTED***');
    expect(result).not.toContain('eyJhbGci');
  });

  it('redacts standalone JWT-like strings', () => {
    const input = 'token=eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abc123';
    const result = redact(input);

    expect(result).toContain('***REDACTED***');
  });

  it('handles null/undefined gracefully', () => {
    expect(redact(null)).toBeNull();
    expect(redact(undefined)).toBeUndefined();
  });

  it('handles primitive values unchanged', () => {
    expect(redact(42)).toBe(42);
    expect(redact(true)).toBe(true);
    expect(redact('hello')).toBe('hello');
  });

  it('prevents infinite recursion with depth limit', () => {
    // Create deeply nested object
    let obj: any = { value: 'deep' };
    for (let i = 0; i < 10; i++) {
      obj = { child: obj };
    }
    // Should not throw
    expect(() => redact(obj)).not.toThrow();
  });
});
