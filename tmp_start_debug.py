import paramiko, time

c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect('152.53.179.54', port=22122, username='root', password='QjIdwdcp5R1Nta3', timeout=10)

c.exec_command('docker rm -f sub2api-debug-test 2>/dev/null')
time.sleep(1)

si, so, se = c.exec_command('cat /home/docker/sub2api-deploy/.env')
env = so.read().decode()
d = {}
for line in env.strip().split('\n'):
    if '=' in line:
        k, v = line.split('=', 1)
        d[k.strip()] = v.strip()

cmd = (
    'docker run -d --name sub2api-debug-test'
    ' --network sub2api-deploy_sub2api-network'
    ' -p 38099:8080'
    ' -e AUTO_SETUP=true'
    ' -e SERVER_HOST=0.0.0.0'
    ' -e SERVER_PORT=8080'
    ' -e SERVER_MODE=release'
    ' -e RUN_MODE=standard'
    ' -e DATABASE_HOST=postgres'
    ' -e DATABASE_PORT=5432'
    ' -e DATABASE_USER=sub2api'
    ' -e DATABASE_PASSWORD=' + d.get('POSTGRES_PASSWORD', '') +
    ' -e DATABASE_DBNAME=sub2api'
    ' -e DATABASE_SSLMODE=disable'
    ' -e REDIS_HOST=redis'
    ' -e REDIS_PORT=6379'
    ' -e REDIS_PASSWORD=' + d.get('REDIS_PASSWORD', '') +
    ' -e REDIS_DB=2'
    ' -e JWT_SECRET=' + d.get('JWT_SECRET', '') +
    ' -e TZ=Asia/Shanghai'
    ' sub2api-debug:test'
)

si, so, se = c.exec_command(cmd)
out = so.read().decode().strip()
err = se.read().decode().strip()
print('ID:', out[:12] if out else 'none')
if err:
    print('Err:', err)

time.sleep(5)

si, so, se = c.exec_command('docker logs sub2api-debug-test --tail 3 2>&1')
print('Logs:', so.read().decode().strip())

si, so, se = c.exec_command('wget -q -O- http://localhost:38099/health 2>&1')
print('Health:', so.read().decode().strip())

c.close()
