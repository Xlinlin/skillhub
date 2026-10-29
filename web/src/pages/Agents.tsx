import { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { listAgents, type AgentPage, type AgentSummary } from '../api/agents';

export default function Agents() {
  const [params, setParams] = useSearchParams();
  return <AgentList key={params.toString()} params={params} change={setParams} />;
}

function AgentList({ params, change }: { params: URLSearchParams; change: (p: URLSearchParams) => void }) {
  const { t } = useTranslation();
  const [query, setQuery] = useState(params.get('q') || '');
  const [result, setResult] = useState<AgentPage<AgentSummary> | null>(null);
  const [error, setError] = useState('');
  const page = Number(params.get('page') || 1);
  const kind = params.get('kind') || '';
  useEffect(() => {
    const controller = new AbortController();
    listAgents(params, controller.signal).then(setResult).catch(e => {
      if (!controller.signal.aborted) setError(e.message);
    });
    return () => controller.abort();
  }, [params]);
  const filter = (q: string, k: string, p = 1) => {
    const next = new URLSearchParams({ page: String(p) });
    if (q) next.set('q', q); if (k) next.set('kind', k);
    change(next);
  };
  return <section className="container agent-page">
    <div className="agent-heading"><div><h1>{t('agents.title')}</h1><p>{t('agents.subtitle')}</p></div>
      <Link to="/publish?type=agent" className="btn btn-primary">{t('agents.publish')}</Link></div>
    <form className="agent-filters" onSubmit={e => { e.preventDefault(); filter(query, kind); }}>
      <input aria-label={t('agents.search')} className="form-input" value={query} maxLength={128} placeholder={t('agents.search')} onChange={e => setQuery(e.target.value)} />
      <select aria-label={t('agents.kind')} className="form-input" value={kind} onChange={e => filter(query, e.target.value)}>
        <option value="">{t('agents.all')}</option><option value="single">{t('agents.single')}</option><option value="suite">{t('agents.suite')}</option>
      </select><button className="btn btn-secondary" type="submit">{t('nav.search')}</button>
    </form>
    {error && <div className="error-box" role="alert">{error}</div>}
    {!result && !error && <p role="status">{t('common.loading')}</p>}
    {result && <><p className="agent-muted">{t('agents.total', { count: result.total })}</p>
      {result.data.length === 0 ? <div className="card agent-empty"><p>{t('agents.empty')}</p><Link to="/publish?type=agent">{t('agents.publish')}</Link></div> :
        <div className="skill-grid">{result.data.map(agent => <Link to={`/agents/${agent.slug}`} key={agent.id} className="card agent-card">
          <div className="agent-heading"><h2>{agent.displayName}</h2><span className="agent-badge">{t(`agents.${agent.kind}`)}</span></div>
          <p className="agent-muted">{agent.slug} · v{agent.latestVersion}</p><p className="agent-summary">{agent.summary || t('agents.noSummary')}</p>
          <div className="agent-tags">{agent.tags.map(tag => <span className="agent-badge" key={tag}>{tag}</span>)}</div>
          <p className="agent-muted">@{agent.ownerHandle} · {new Date(agent.updatedAt).toLocaleDateString()}</p>
        </Link>)}</div>}
      <div className="agent-pagination"><button className="btn btn-secondary" disabled={page <= 1} onClick={() => filter(query, kind, page - 1)}>{t('agents.previous')}</button>
        <span>{page} / {Math.max(1, Math.ceil(result.total / result.pageSize))}</span>
        <button className="btn btn-secondary" disabled={page * result.pageSize >= result.total} onClick={() => filter(query, kind, page + 1)}>{t('agents.next')}</button></div>
    </>}
  </section>;
}
