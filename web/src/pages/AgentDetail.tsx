import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { agentDownloadURL, deleteAgent, getAgent, getAgentFile, listAgentVersions, type AgentDetail as Detail, type AgentVersionSummary, type AgentPage } from '../api/agents';
import { useAuth } from '../hooks/useAuth';

export default function AgentDetail() {
  const { slug = '' } = useParams(); const [params] = useSearchParams();
  const version = params.get('version') || 'latest';
  return <AgentContent key={`${slug}/${version}`} slug={slug} requestedVersion={version} />;
}
function AgentContent({ slug, requestedVersion }: { slug: string; requestedVersion: string }) {
  const { t } = useTranslation(); const { user } = useAuth(); const navigate = useNavigate();
  const [detail, setDetail] = useState<Detail | null>(null); const [error, setError] = useState('');
  const [selectedPath, setSelectedPath] = useState(''); const [confirmDelete, setConfirmDelete] = useState(false); const [deleting, setDeleting] = useState(false);
  useEffect(() => {
    const c = new AbortController(); getAgent(slug, requestedVersion, c.signal).then(setDetail).catch(e => { if (!c.signal.aborted) setError(e.message); });
    return () => c.abort();
  }, [slug, requestedVersion]);
  const remove = async () => { setDeleting(true); try { await deleteAgent(slug); navigate('/agents'); } catch (e) { setError(e instanceof Error ? e.message : String(e)); setDeleting(false); } };
  if (!detail) return <div className="container agent-page">{error ? <div className="error-box" role="alert">{error}</div> : <p role="status">{t('common.loading')}</p>}<Link to="/agents">{t('agents.back')}</Link></div>;
  const { agent, version } = detail; const isOwner = user?.id === agent.ownerId;
  return <section className="container agent-page">
    <Link to="/agents">← {t('agents.back')}</Link>
    <div className="agent-heading"><div><h1>{agent.displayName}</h1><p className="agent-muted">@{agent.ownerHandle} · {agent.slug} · {t(`agents.${agent.kind}`)}</p></div>
      <a data-testid="agent-download" className="btn btn-primary" href={agentDownloadURL(slug, version.version)}>{t('agents.download')} · {version.version}</a></div>
    <p className="agent-summary">{agent.summary || t('agents.noSummary')}</p>
    <div className="agent-tags">{agent.tags.map(tag => <span className="agent-badge" key={tag}>{tag}</span>)}</div>
    <div className="agent-actions">{isOwner && <Link className="btn btn-secondary" to={`/publish?type=agent&slug=${encodeURIComponent(slug)}`}>{t('agents.newVersion')}</Link>}
      {(isOwner || user?.role === 'admin') && <button className="btn btn-secondary" onClick={() => setConfirmDelete(true)}>{t('agents.delete')}</button>}</div>
    {confirmDelete && <div className="error-box"><p>{t('agents.deleteConfirm')}</p><div className="agent-actions"><button className="btn btn-secondary" onClick={() => setConfirmDelete(false)} disabled={deleting}>{t('agents.cancel')}</button><button className="btn btn-primary" onClick={remove} disabled={deleting}>{t('agents.confirmDelete')}</button></div></div>}
    {error && <div className="error-box" role="alert">{error}</div>}
    <div className="agent-detail-grid"><div>
      <section className="card agent-section"><h2>{t('agents.instructions')}</h2><pre className="agent-prose">{version.instructions || t('agents.noInstructions')}</pre></section>
      {version.changelog && <section className="card agent-section"><h2>{t('agents.changelog')}</h2><pre className="agent-prose">{version.changelog}</pre></section>}
      <section className="card agent-section"><h2>{t('agents.files')} ({version.files.length})</h2>
        <ul className="agent-file-list">{version.files.map(file => <li key={file.path}><button className={selectedPath === file.path ? 'agent-file active' : 'agent-file'} onClick={() => setSelectedPath(file.path)} style={{ paddingLeft: `${12 + (file.path.split('/').length - 1) * 12}px` }}><span>{file.path}</span><small>{file.size.toLocaleString()} B</small></button></li>)}</ul>
        {selectedPath && <AgentPreview key={`${version.id}/${selectedPath}`} slug={slug} version={version.version} path={selectedPath} />}
      </section>
    </div><aside>
      <section className="card agent-section"><h2>{t('agents.versions')}</h2><VersionPicker slug={slug} selected={version.version} latest={agent.latestVersion} /></section>
      <section className="card agent-section"><h2>{t('agents.archive')}</h2><p>{(version.archiveSize / 1024).toFixed(1)} KiB</p><p className="agent-muted">SHA256</p><code className="agent-hash">{version.archiveSHA256}</code><p className="agent-muted">{new Date(version.createdAt).toLocaleString()}</p></section>
    </aside></div>
  </section>;
}
function VersionPicker({ slug, selected, latest }: { slug: string; selected: string; latest: string }) {
  const { t } = useTranslation(); const [page, setPage] = useState(1); const [result, setResult] = useState<AgentPage<AgentVersionSummary> | null>(null); const [error, setError] = useState(''); const navigate = useNavigate();
  useEffect(() => { const c = new AbortController(); listAgentVersions(slug, page, c.signal).then(setResult).catch(e => { if (!c.signal.aborted) setError(e.message); }); return () => c.abort(); }, [slug, page]);
  return <>{error && <p role="alert">{error}</p>}<select className="form-input" aria-label={t('agents.versions')} value={selected} onChange={e => navigate(`/agents/${slug}?version=${encodeURIComponent(e.target.value)}`)}>
    {!result?.data.some(v => v.version === selected) && <option value={selected}>{selected}</option>}
    {result?.data.map(v => <option key={v.id} value={v.version}>{v.version}{v.version === latest ? ` (${t('agents.latest')})` : ''}</option>)}</select>
    {result && result.total > 20 && <div className="agent-pagination"><button className="btn btn-secondary btn-small" disabled={page <= 1} onClick={() => setPage(page - 1)}>{t('agents.previous')}</button><span>{page}</span><button className="btn btn-secondary btn-small" disabled={page * 20 >= result.total} onClick={() => setPage(page + 1)}>{t('agents.next')}</button></div>}
  </>;
}
function AgentPreview({ slug, version, path }: { slug: string; version: string; path: string }) {
  const { t } = useTranslation(); const [text, setText] = useState<string | null>(null); const [error, setError] = useState('');
  useEffect(() => { const c = new AbortController(); getAgentFile(slug, version, path, c.signal).then(setText).catch(e => { if (!c.signal.aborted) setError(e.message); }); return () => c.abort(); }, [slug, version, path]);
  return <div className="agent-preview"><h3>{path}</h3>{error ? <p role="alert">{error}</p> : text === null ? <p>{t('common.loading')}</p> : <pre data-testid="agent-preview">{text}</pre>}</div>;
}
