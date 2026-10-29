import { useTranslation } from 'react-i18next';
import CodeBlock from './CodeBlock';

export default function AgentPublishGuide() {
  const { t } = useTranslation();
  const commandStyle = { whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', paddingRight: 80 } as const;
  const flags = '--type agent --kind suite --slug my-agent-suite --name "My Agent Suite" --version 1.0.0';
  const api = `${window.location.origin}/api/v1/agents`;
  const fields = '--form-string "slug=my-agent-suite" --form-string "displayName=My Agent Suite" --form-string "kind=suite" --form-string "version=1.0.0"';
  const endpoints = [
    ['POST', '/api/v1/agents', 'publish'],
    ['GET', '/api/v1/agents', 'list'],
    ['GET', '/api/v1/agents/:slug?version=1.0.0', 'detail'],
    ['GET', '/api/v1/agents/:slug/versions', 'versions'],
    ['GET', '/api/v1/agents/:slug/file?version=1.0.0&path=README.md', 'file'],
    ['GET', '/api/v1/agents/:slug/download?version=1.0.0', 'download'],
    ['DELETE', '/api/v1/agents/:slug', 'delete'],
  ];
  return <div className="agent-publish-guide">
    <section className="card agent-section">
      <h2>{t('publish.cli_title')}</h2>
      <p>{t('agentPublishGuide.login', { registry: window.location.origin })}</p>
      <CodeBlock command="skillhub login" style={commandStyle} />
      <p>{t('agentPublishGuide.token')}</p>
      <CodeBlock command="skillhub admin create-token --user YOUR_HANDLE --label CLI" style={commandStyle} />
      <h3>{t('agentPublishGuide.directory')}</h3>
      <CodeBlock command={`skillhub publish ./my-agent-suite ${flags}`} style={commandStyle} />
      <h3>{t('agentPublishGuide.zip')}</h3>
      <CodeBlock command={`skillhub publish ./my-agent-suite.zip ${flags}`} style={commandStyle} />
      <p>{t('agentPublishGuide.options')}</p>
      <p>{t('agentPublishGuide.snapshot')}</p>
    </section>
    <section className="card agent-section">
      <h2>{t('publish.curl_title')}</h2>
      <p>{t('agentPublishGuide.shell')}</p>
      <h3>{t('agentPublishGuide.zip')}</h3>
      <CodeBlock prefix="" command={`curl.exe --fail-with-body "${api}" -H "Authorization: Bearer YOUR_TOKEN" --form-string "mode=archive" ${fields} --form "archive=@./my-agent-suite.zip"`} style={commandStyle} />
      <h3>{t('agentPublishGuide.files')}</h3>
      <p>{t('agentPublishGuide.pathsFile')}</p>
      <CodeBlock prefix="" command={'[".agents/pm.md","README.md"]'} style={commandStyle} />
      <CodeBlock prefix="" command={`curl.exe --fail-with-body "${api}" -H "Authorization: Bearer YOUR_TOKEN" --form-string "mode=files" ${fields} --form "paths=<./paths.json" --form "files=@./my-agent-suite/.agents/pm.md" --form "files=@./my-agent-suite/README.md"`} style={commandStyle} />
      <p>{t('agentPublishGuide.protocol')}</p>
      <p>{t('agents.limits')}</p>
    </section>
    <section className="card agent-section">
      <h2>{t('publish.api_title')}</h2>
      <div style={{ overflowX: 'auto' }}><table style={{ minWidth: 760, width: '100%', textAlign: 'left', borderSpacing: '8px 12px' }}><thead><tr><th>{t('publish.method')}</th><th>{t('publish.endpoint')}</th><th>{t('publish.description')}</th></tr></thead>
        <tbody>{endpoints.map(([method, path, key]) => <tr key={key}><td>{method}</td><td><code>{path}</code></td><td>{t(`agentPublishGuide.api_${key}`)}</td></tr>)}</tbody>
      </table></div>
      <p>{t('agentPublishGuide.errors')}</p>
    </section>
  </div>;
}
