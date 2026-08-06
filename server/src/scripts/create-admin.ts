import 'reflect-metadata';
import * as bcrypt from 'bcrypt';
import { AppDataSource } from '../data-source';
import { User, UserRole } from '../entities/user.entity';

/**
 * Create (or reset the password of) an admin user.
 *
 * Usage:
 *   pnpm create:admin <username> <password> [role]
 *   ADMIN_USERNAME=admin ADMIN_PASSWORD=secret pnpm create:admin
 *
 * role defaults to SuperAdmin. Passwords are never logged.
 */
async function main() {
  const username = process.argv[2] || process.env.ADMIN_USERNAME;
  const password = process.argv[3] || process.env.ADMIN_PASSWORD;
  const roleArg = process.argv[4] || process.env.ADMIN_ROLE || UserRole.SuperAdmin;

  if (!username || !password) {
    console.error(
      'Usage: pnpm create:admin <username> <password> [role]\n' +
        '   or: ADMIN_USERNAME=.. ADMIN_PASSWORD=.. pnpm create:admin',
    );
    process.exit(1);
  }

  if (username.length < 3 || username.length > 20) {
    console.error('username must be 3-20 characters');
    process.exit(1);
  }
  if (password.length < 6 || password.length > 72) {
    console.error('password must be 6-72 characters');
    process.exit(1);
  }

  const role = Object.values(UserRole).includes(roleArg as UserRole)
    ? (roleArg as UserRole)
    : UserRole.SuperAdmin;

  await AppDataSource.initialize();
  try {
    const repo = AppDataSource.getRepository(User);
    const hash = await bcrypt.hash(password, 10);

    const existing = await repo.findOne({ where: { username } });
    if (existing) {
      existing.password = hash;
      existing.role = role;
      await repo.save(existing);
      console.log(`Updated existing user "${username}" (role=${role}).`);
    } else {
      const user = repo.create({ username, password: hash, role });
      await repo.save(user);
      console.log(`Created user "${username}" (role=${role}).`);
    }
  } finally {
    await AppDataSource.destroy();
  }
}

main().catch((err) => {
  console.error('Failed to create admin user:', err instanceof Error ? err.message : err);
  process.exit(1);
});
