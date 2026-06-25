import paramiko

c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect('152.53.179.54', port=22122, username='root', password='QjIdwdcp5R1Nta3', timeout=10)

si, so, se = c.exec_command('docker logs sub2api-debug-test --tail 100 2>&1 | grep CC-DEBUG')
out = so.read().decode().strip()
print(out if out else 'no CC-DEBUG')

print('\n--- recent 503s ---')
si, so, se = c.exec_command('docker logs sub2api-debug-test --tail 200 2>&1 | grep "status_code.: 503"')
out = so.read().decode().strip()
print(out if out else 'no 503s')

c.close()
