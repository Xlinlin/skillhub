"""Exercise the real CLI and curl against an isolated local SQLite instance.

Usage: python scripts/test-agent-cli.py /absolute/path/skillhub.exe [--keep-server]
Requires curl (curl.exe on Windows). Test credentials never go to stdout.
"""
import hashlib
import io
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.request
import zipfile

binary = str(Path(sys.argv[1]).resolve())
root = Path('.superpowers/sdd/2026-10-29-agent-cli-publish') / ('run-' + secrets.token_hex(4))
root = root.resolve()
root.mkdir(parents=True)
with socket.socket() as sock:
    sock.bind(('127.0.0.1', 0))
    port = sock.getsockname()[1]
base = f'http://127.0.0.1:{port}'
env = dict(os.environ, SKILLHUB_DATABASE_URL=str(root / 'test.db'), SKILLHUB_DB_DRIVER='sqlite',
           SKILLHUB_HOST='127.0.0.1', SKILLHUB_PORT=str(port), SKILLHUB_BASE_URL=base,
           SKILLHUB_GIT_PATH=str(root / 'repos'), SKILLHUB_AGENT_PATH=str(root / 'agents'),
           SKILLHUB_SEARCH_PATH=str(root / 'search'), GIN_MODE='release', NO_PROXY='127.0.0.1,localhost')
config = root / 'server.yaml'
config.write_text('search:\n  index_path: "' + str(root / 'search').replace('\\', '/') + '"\n', encoding='utf-8')
env['SKILLHUB_CONFIG'] = str(config)
for key in ('SKILLHUB_ADMIN_USER', 'SKILLHUB_ADMIN_PASSWORD'):
    env.pop(key, None)
checks = []

def run(args, *, process_env=env, stdin=None, expected=0):
    result = subprocess.run(args, env=process_env, input=stdin, text=True, encoding='utf-8',
                            capture_output=True, timeout=60)
    if expected == 0 and result.returncode != 0:
        raise AssertionError(f'command failed (exit {result.returncode}): {result.stderr[:500]}')
    if expected != 0 and result.returncode == 0:
        raise AssertionError('expected a nonzero exit')
    return result

password = secrets.token_urlsafe(24)
run([binary, 'admin', 'create-user', '--handle', 'cli-owner', '--role', 'user', '--password', password])
run([binary, 'admin', 'create-user', '--handle', 'cli-other', '--role', 'user'])

def token(handle):
    result = run([binary, 'admin', 'create-token', '--user', handle, '--label', 'isolated-e2e'])
    return result.stdout.strip().splitlines()[-1]

owner, other = token('cli-owner'), token('cli-other')
log = (root / 'server.log').open('w', encoding='utf-8')
server = subprocess.Popen([binary, 'serve'], env=env, stdout=log, stderr=log,
                          creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
try:
    for _ in range(100):
        try:
            urllib.request.urlopen(base + '/api/v1/agents', timeout=1).close()
            break
        except (OSError, urllib.error.URLError):
            if server.poll() is not None:
                raise AssertionError('isolated server exited; see server.log')
            time.sleep(.1)
    else:
        raise AssertionError('isolated server did not become ready')
    cli_env = dict(env, USERPROFILE=str(root / 'home'), HOME=str(root / 'home'))
    Path(cli_env['USERPROFILE']).mkdir()
    run([binary, 'login'], process_env=cli_env, stdin=base + '\n' + owner + '\n')
    checks.append('real login uses an isolated HOME')
    source = root / 'my-agent-suite'
    fixture = {'.agents/same.md': '产品需求\n'.encode(), '.claude/same.md': b'\x00\x01\xff',
               '设计/需求.md': '设计→开发→测试\n'.encode(), 'README.md': b'# Suite\n', 'empty.txt': b''}
    for path, content in fixture.items():
        dest = source / path
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_bytes(content)
    usage = root / 'USAGE.md'
    usage.write_text('Run in your Agent tool.\n', encoding='utf-8')
    flags = ['--type', 'agent', '--kind', 'suite', '--slug', 'cli-suite', '--name', 'CLI Suite']
    def publish(path, version, **kw):
        return run([binary, 'publish', str(path), *flags, '--version', version, '--instructions-file', str(usage),
                    '--tags', 'product,design'], process_env=cli_env, **kw)
    publish(source, '1.0.0')
    def get(path):
        with urllib.request.urlopen(base + '/api/v1/agents/' + path, timeout=10) as response:
            return response.read(), response.headers
    def snapshot(version, contents):
        archive, headers = get('cli-suite/download?version=' + version)
        assert hashlib.sha256(archive).hexdigest() == headers['X-Content-SHA256']
        with zipfile.ZipFile(io.BytesIO(archive)) as z:
            assert {i.filename: z.read(i) for i in z.infolist() if not i.is_dir()} == contents
        return archive
    old = snapshot('1.0.0', fixture)
    checks.append('directory first publish: hidden/Chinese/same-basename/binary/empty files exact')
    fixture['README.md'] = b'# Version two\n'
    zip_path = root / 'my-agent-suite.zip'
    with zipfile.ZipFile(zip_path, 'w') as z:
        for path, content in fixture.items():
            z.writestr(path, content)
    publish(zip_path, '1.1.0')
    assert snapshot('1.1.0', fixture) == zip_path.read_bytes()
    assert get('cli-suite/download?version=1.0.0')[0] == old
    checks.append('ZIP append: exact ZIP SHA256 and immutable old snapshot')
    assert '409' in publish(source, '1.1.0', expected=1).stderr
    config_path = Path(cli_env['USERPROFILE']) / '.skillhub/config.yaml'
    saved = config_path.read_text(encoding='utf-8')
    for bad, status in ((other, '403'), ('invalid-token', '401')):
        config_path.write_text('registry: ' + base + '\ntoken: ' + bad + '\n', encoding='utf-8')
        assert status in publish(source, '1.2.0', expected=1).stderr
    config_path.write_text(saved, encoding='utf-8')
    run([binary, 'publish', str(source), '--type', 'typo'], process_env=cli_env, expected=1)
    checks.append('CLI conflicts 409 / nonowner 403 / invalid Token 401 / unknown type: nonzero exit')
    skill = root / 'skill'
    skill.mkdir()
    (skill / 'SKILL.md').write_text('---\nname: cli-skill\ndescription: CLI regression\n---\n# Skill\n', encoding='utf-8')
    run([binary, 'publish', str(skill), '--slug', 'cli-skill', '--version', '1.0.0'], process_env=cli_env)
    urllib.request.urlopen(base + '/api/v1/skills/cli-skill', timeout=10).close()
    checks.append('legacy Skill publishing defaults to the original endpoint')
    curl = 'curl.exe' if os.name == 'nt' else 'curl'
    def api(mode, version, parts, expected):
        args = [curl, '--silent', '--show-error', '--fail-with-body', base + '/api/v1/agents', '-H',
                'Authorization: Bearer ' + owner, '--form-string', 'mode=' + mode,
                '--form-string', 'slug=my-agent-suite', '--form-string', 'displayName=My Agent Suite',
                '--form-string', 'kind=suite', '--form-string', 'version=' + version, *parts, '-w', '\n%{http_code}']
        response = run(args).stdout
        assert response.rstrip().endswith(str(expected)), response[-100:]
    api('archive', '1.0.0', ['--form', 'archive=@' + str(zip_path)], 201)
    paths_file = root / 'paths.json'
    paths_file.write_text('[".agents/pm.md","README.md"]', encoding='utf-8')
    api('files', '1.1.0', ['--form', 'paths=<' + str(paths_file),
                         '--form', 'files=@' + str(source / '.agents/same.md'),
                         '--form', 'files=@' + str(source / 'README.md')], 200)
    checks.append('real curl archive 201 and files+paths 200 using guide protocol')
    if os.name == 'nt':
        # Execute the copyable syntax through both Windows shells, not only argv.
        for shell in ('powershell.exe', 'pwsh.exe'):
            shell_slug = 'shell-' + shell.split('.')[0]
            def quoted(value):
                return "'" + str(value).replace("'", "''") + "'"
            command = 'curl.exe --silent --show-error --fail-with-body ' + quoted(base + '/api/v1/agents')
            command += ' -H "Authorization: Bearer $env:AGENT_TEST_TOKEN"'
            for field in ('mode=files', 'slug=' + shell_slug, 'displayName=My Agent Suite', 'kind=suite', 'version=1.0.0'):
                command += ' --form-string ' + quoted(field)
            for field in ('paths=<' + str(paths_file), 'files=@' + str(source / '.agents/same.md'), 'files=@' + str(source / 'README.md')):
                command += ' --form ' + quoted(field)
            command += ' --output ' + quoted(root / (shell_slug + '.json')) + ' --write-out "%{http_code}"; exit $LASTEXITCODE'
            response = run([shell, '-NoProfile', '-NonInteractive', '-Command', command],
                           process_env=dict(env, AGENT_TEST_TOKEN=owner))
            assert response.stdout.strip() == '201', response.stdout
        checks.append('copyable files API syntax: Windows PowerShell 5.1 and PowerShell 7 both return 201')
    if os.name == 'nt' and '--keep-server' in sys.argv:
        secret_file = root / 'browser-password.xml'
        ps = "$v=[Console]::In.ReadToEnd(); ConvertTo-SecureString -String $v -AsPlainText -Force | Export-Clixml -LiteralPath '" + str(secret_file).replace("'", "''") + "'"
        run(['pwsh.exe', '-NoProfile', '-NonInteractive', '-Command', ps], stdin=password)
    result = {'baseUrl': base, 'root': str(root), 'serverPid': server.pid, 'checks': checks}
    (root / 'results.json').write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding='utf-8')
    print(json.dumps(result, ensure_ascii=False))
finally:
    if '--keep-server' not in sys.argv or sys.exc_info()[0] is not None:
        server.terminate()
        server.wait(timeout=15)
    log.close()
