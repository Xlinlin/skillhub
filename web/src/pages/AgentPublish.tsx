import { useEffect, useState } from 'react';
import { Link, Navigate, useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { getAgent, publishAgent, type AgentDetail } from '../api/agents';
import { useAuth } from '../hooks/useAuth';
import AgentPublishGuide from '../components/AgentPublishGuide';

export default function AgentPublish() {
  const { user, loading } = useAuth(); const location = useLocation(); const [params] = useSearchParams(); const { t } = useTranslation();
  if (loading) return <p className="container agent-page">{t('common.loading')}</p>;
  if (!user) return <Navigate replace to={`/login?redirect=${encodeURIComponent(location.pathname + location.search)}`} />;
  const slug = params.get('slug') || '';
  return slug ? <ExistingAgent key={slug} slug={slug} userId={user.id} /> : <AgentPublishPanel key="new" />;
}
function ExistingAgent({ slug, userId }: { slug: string; userId: string }) {
  const { t } = useTranslation(); const [detail, setDetail] = useState<AgentDetail | null>(null); const [error, setError] = useState('');
  useEffect(() => { const c = new AbortController(); getAgent(slug, 'latest', c.signal).then(setDetail).catch(e => { if (!c.signal.aborted) setError(e.message); }); return () => c.abort(); }, [slug]);
  if (error) return <div className="container agent-page error-box" role="alert">{error}</div>;
  if (!detail) return <p className="container agent-page">{t('common.loading')}</p>;
  if (detail.agent.ownerId !== userId) return <div className="container agent-page error-box" role="alert">{t('agents.ownerOnly')}</div>;
  return <AgentPublishPanel existing={detail} />;
}
function AgentPublishPanel({ existing }: { existing?: AgentDetail }) {
  const { t } = useTranslation();
  const [tab, setTab] = useState<'upload' | 'cli'>('upload');
  return <section className="container agent-page agent-publish">
    <Link to={existing ? `/agents/${existing.agent.slug}` : '/agents'}>← {t('agents.back')}</Link>
    <h1>{t(existing ? 'agents.newVersion' : 'agents.publish')}</h1><p className="agent-muted">{t('agents.publishHelp')}</p>
    {existing && <p>{t('agents.currentVersion')}: {existing.agent.latestVersion} · {t('agents.snapshotHelp')}</p>}
    <div className="tab-bar">
      <button type="button" className={`tab-btn ${tab === 'upload' ? 'active' : ''}`} aria-pressed={tab === 'upload'} onClick={() => setTab('upload')}>{t('publish.tab_upload')}</button>
      <button type="button" className={`tab-btn ${tab === 'cli' ? 'active' : ''}`} aria-pressed={tab === 'cli'} onClick={() => setTab('cli')}>{t('publish.tab_cli')}</button>
    </div>
    <div hidden={tab !== 'upload'}><AgentForm existing={existing} /></div>
    {tab === 'cli' && <AgentPublishGuide />}
  </section>;
}
function AgentForm({ existing }: { existing?: AgentDetail }) {
  const { t } = useTranslation(); const navigate = useNavigate();
  const [values, setValues] = useState({ slug: existing?.agent.slug || '', displayName: existing?.agent.displayName || '', kind: existing?.agent.kind || 'suite', version: '', summary: existing?.agent.summary || '', instructions: existing?.version.instructions || '', changelog: '', tags: existing?.agent.tags.join(', ') || '' });
  const [mode, setMode] = useState<'archive' | 'files' | 'folder'>('archive'); const [files, setFiles] = useState<File[]>([]); const [error, setError] = useState(''); const [busy, setBusy] = useState(false);
  const field = (name: keyof typeof values) => ({ name, id: `agent-${name}`, value: values[name], onChange: (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>) => setValues({ ...values, [name]: e.target.value }) });
  const submit = async (e: React.FormEvent) => {
    e.preventDefault(); if (busy) return; setError('');
    if (!files.length) { setError(t('agents.chooseRequired')); return; }
    if (mode === 'archive' && (files.length !== 1 || files[0].size > 50 * 1024 * 1024)) { setError(t('agents.limits')); return; }
    if (mode !== 'archive' && (files.length > 500 || files.some(f => f.size > 10 * 1024 * 1024) || files.reduce((s, f) => s + f.size, 0) > 50 * 1024 * 1024)) { setError(t('agents.limits')); return; }
    const body = new FormData();
    for (const [name, value] of Object.entries(values)) if (name !== 'tags') body.append(name, value);
    body.append('tags', JSON.stringify(values.tags.split(',').map(v => v.trim()).filter(Boolean)));
    body.append('mode', mode === 'archive' ? 'archive' : 'files');
    if (mode === 'archive') body.append('archive', files[0]);
    else {
      const paths = files.map(file => mode === 'folder' ? file.webkitRelativePath.split('/').slice(1).join('/') : file.name);
      body.append('paths', JSON.stringify(paths)); files.forEach(file => body.append('files', file));
    }
    setBusy(true);
    try { const result = await publishAgent(body); navigate(`/agents/${result.agent.slug}?version=${encodeURIComponent(result.version.version)}`); }
    catch (err) { setError(err instanceof Error ? err.message : String(err)); setBusy(false); }
  };
  return <form onSubmit={submit} className="card agent-section">
      <fieldset disabled={busy} className="agent-fieldset"><div className="agent-form-grid">
        <div><label className="form-label" htmlFor="agent-slug">Slug *</label><input className="form-input" {...field('slug')} required readOnly={!!existing} maxLength={128} pattern="[a-z0-9](([a-z0-9]|-){0,126}[a-z0-9])?" placeholder="product-delivery" /></div>
        <div><label className="form-label" htmlFor="agent-displayName">{t('agents.name')} *</label><input className="form-input" {...field('displayName')} required maxLength={256} /></div>
        <div><label className="form-label" htmlFor="agent-kind">{t('agents.kind')} *</label><select className="form-input" {...field('kind')} disabled={!!existing}><option value="suite">{t('agents.suite')}</option><option value="single">{t('agents.single')}</option></select></div>
        <div><label className="form-label" htmlFor="agent-version">{t('agents.version')} *</label><input className="form-input" {...field('version')} required maxLength={32} pattern="(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)" placeholder="1.0.0" /></div>
      </div>
      <label className="form-label" htmlFor="agent-summary">{t('agents.summary')}</label><textarea className="form-input" {...field('summary')} rows={3} maxLength={2000} />
      <label className="form-label" htmlFor="agent-tags">{t('agents.tags')}</label><input className="form-input" {...field('tags')} placeholder="product, design, development, testing" />
      <label className="form-label" htmlFor="agent-instructions">{t('agents.instructions')}</label><textarea className="form-input" {...field('instructions')} rows={7} maxLength={65536} placeholder={t('agents.instructionsHelp')} />
      <label className="form-label" htmlFor="agent-changelog">{t('agents.changelog')}</label><textarea className="form-input" {...field('changelog')} rows={3} maxLength={16384} />
      <div className="agent-upload"><h2>{t('agents.upload')}</h2><div className="agent-actions">{(['archive', 'files', 'folder'] as const).map(m => <label key={m} className="agent-mode"><input type="radio" name="uploadMode" value={m} checked={mode === m} onChange={() => { setMode(m); setFiles([]); }} /> {t(`agents.mode_${m}`)}</label>)}</div>
        <label className="form-label" htmlFor="agent-upload">{t(`agents.mode_${mode}`)} *</label>
        <input key={mode} id="agent-upload" type="file" accept={mode === 'archive' ? '.zip,application/zip' : undefined} multiple={mode !== 'archive'} {...(mode === 'folder' ? { webkitdirectory: '' } : {})} onChange={e => setFiles(Array.from(e.target.files || []))} />
        <p className="agent-muted">{t('agents.limits')}</p><p className="agent-muted">{t(mode === 'archive' ? 'agents.zipHelp' : 'agents.folderHelp')}</p>
        {files.length > 0 && <details className="agent-selection"><summary>{t('agents.selected', { count: files.length })} · {(files.reduce((s, f) => s + f.size, 0) / 1024).toFixed(1)} KiB</summary><ul>{files.map((f, i) => <li key={i}>{f.webkitRelativePath || f.name}</li>)}</ul></details>}
      </div>
      {error && <div className="error-box" role="alert">{error}</div>}
      <button type="submit" className="btn btn-primary" disabled={busy}>{t(busy ? 'agents.publishing' : 'agents.submit')}</button>
      </fieldset>
    </form>;
}
