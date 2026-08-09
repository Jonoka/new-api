import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const root = dirname(fileURLToPath(import.meta.url));
const readSource = (...parts) => readFileSync(resolve(root, ...parts), 'utf8');

test('classic root user can create another super administrator', () => {
  const source = readSource('components/table/users/modals/AddUserModal.jsx');

  assert.match(source, /field='role'/);
  assert.match(source, /label: t\('超级管理员'\), value: 100/);
  assert.match(source, /canAssignAdminRoles = isRoot\(\)/);
});

test('classic role actions explicitly grant and revoke root access', () => {
  const tableSource = readSource('components/table/users/UsersTable.jsx');
  const columnsSource = readSource(
    'components/table/users/UsersColumnDefs.jsx',
  );

  assert.match(tableSource, /'promote_root'/);
  assert.match(tableSource, /'demote_root'/);
  assert.match(columnsSource, /record\.role === 10/);
  assert.match(columnsSource, /record\.role === 100/);
  assert.match(columnsSource, /record\.id !== currentUserId/);
});
