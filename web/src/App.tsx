import { useEffect, useState, useCallback } from 'react';
import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { AuthContext } from './hooks/useAuth';
import { whoami, type User } from './api/auth';
import Layout from './components/Layout';
import Home from './pages/Home';
import Skills from './pages/Skills';
import SkillDetail from './pages/SkillDetail';
import Search from './pages/Search';
import PublishHub, { LegacyAgentPublish } from './pages/PublishHub';
import Login from './pages/Login';
import Agents from './pages/Agents';
import AgentDetail from './pages/AgentDetail';

export default function App() {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    setLoading(true);
    const u = await whoami();
    setUser(u);
    setLoading(false);
  }, []);

  useEffect(() => {
    let active = true;
    whoami().then(u => {
      if (active) { setUser(u); setLoading(false); }
    });
    return () => { active = false; };
  }, []);

  return (
    <AuthContext.Provider value={{ user, loading, refresh }}>
      <BrowserRouter>
        <Routes>
          <Route element={<Layout />}>
            <Route path="/" element={<Home />} />
            <Route path="/skills" element={<Skills />} />
            <Route path="/skills/:slug" element={<SkillDetail />} />
            <Route path="/search" element={<Search />} />
            <Route path="/publish" element={<PublishHub />} />
            <Route path="/login" element={<Login />} />
            <Route path="/agents" element={<Agents />} />
            <Route path="/agents/publish" element={<LegacyAgentPublish />} />
            <Route path="/agents/:slug" element={<AgentDetail />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </AuthContext.Provider>
  );
}
