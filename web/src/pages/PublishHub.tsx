import { Link, Navigate, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import SkillPublish from './Publish';
import AgentPublish from './AgentPublish';

export default function PublishHub() {
  const { t } = useTranslation();
  const [params] = useSearchParams();
  const type = params.get('type');
  if (type !== 'skill' && type !== 'agent') {
    return <section className="container publish-hub">
      <h1>{t('publishHub.title')}</h1>
      <p className="publish-hub-subtitle">{t('publishHub.subtitle')}</p>
      <div className="publish-choice-grid">
        {(['skill', 'agent'] as const).map(kind => <Link key={kind} data-testid={`publish-${kind}-choice`} to={`/publish?type=${kind}`} className="card publish-choice">
          <h2>{t(`publishHub.${kind}Title`)}</h2>
          <p>{t(`publishHub.${kind}Description`)}</p>
          <span className="btn btn-primary">{t(`publishHub.${kind}Action`)} →</span>
        </Link>)}
      </div>
    </section>;
  }
  return <>
    <nav className="container publish-type-nav" aria-label={t('publishHub.typeLabel')}>
      <span>{t('publishHub.typeLabel')}</span>
      <Link className={`filter-btn ${type === 'skill' ? 'active' : ''}`} aria-current={type === 'skill' ? 'page' : undefined} to="/publish?type=skill">{t('publishHub.skillAction')}</Link>
      <Link className={`filter-btn ${type === 'agent' ? 'active' : ''}`} aria-current={type === 'agent' ? 'page' : undefined} to={type === 'agent' ? `/publish?${params}` : '/publish?type=agent'}>{t('publishHub.agentAction')}</Link>
    </nav>
    {type === 'skill' ? <SkillPublish /> : <AgentPublish />}
  </>;
}

export function LegacyAgentPublish() {
  const [params] = useSearchParams();
  const next = new URLSearchParams(params);
  next.set('type', 'agent');
  return <Navigate replace to={`/publish?${next}`} />;
}
