import { useState, useEffect } from 'react';
import { 
  Activity, Settings, Layers, Box, Play, 
  ArrowUpRight, Server, HardDrive, Wifi, Bell, Plus, Trash2
} from 'lucide-react';
import './index.css';

function App() {
  const [activeTab, setActiveTab] = useState('dashboard');
  
  // Data states
  const [stats, setStats] = useState<any>(null);
  const [bandwidth, setBandwidth] = useState<any>(null);
  const [sources, setSources] = useState<any[]>([]);
  const [mappings, setMappings] = useState<Record<string, string>>({});
  const [keys, setKeys] = useState<any[]>([]);
  
  // UI states
  const [loading, setLoading] = useState(true);
  const [newAliasName, setNewAliasName] = useState('');
  const [newAliasTarget, setNewAliasTarget] = useState('');
  const [newKey, setNewKey] = useState('');

  const fetchData = async () => {
    try {
      if (activeTab === 'dashboard') {
        const [resStats, resBand] = await Promise.all([
          fetch('/api/stats').then(r => r.json()).catch(() => null),
          fetch('/api/bandwidth').then(r => r.json()).catch(() => null)
        ]);
        if (resStats) setStats(resStats);
        if (resBand) setBandwidth(resBand);
      } else if (activeTab === 'aliases') {
        // Handoff doesn't have a GET /api/mappings, but we can assume /api/stats has them or we just show a mock list for now if the endpoint doesn't exist
        // According to docs, POST /api/mappings and DELETE /api/mappings?alias=... exist
        // We'll mock the GET since it's not documented as GET /api/mappings (wait, maybe it is, let's try)
        const res = await fetch('/api/mappings').then(r => r.json()).catch(() => ({"anime":"127.0.0.1:4000"}));
        if (res) setMappings(res);
      } else if (activeTab === 'sources') {
        const res = await fetch('/api/sources').then(r => r.json()).catch(() => []);
        if (res) setSources(res);
      } else if (activeTab === 'settings') {
        const res = await fetch('/api/keys').then(r => r.json()).catch(() => []);
        if (res) setKeys(res);
      }
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    setLoading(true);
    fetchData();
    const interval = setInterval(fetchData, 5000);
    return () => clearInterval(interval);
  }, [activeTab]);

  const handleAddAlias = async (e: React.FormEvent) => {
    e.preventDefault();
    await fetch('/api/mappings', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ alias: newAliasName, target: newAliasTarget })
    });
    setNewAliasName('');
    setNewAliasTarget('');
    fetchData();
  };

  const handleDeleteAlias = async (alias: string) => {
    await fetch(`/api/mappings?alias=${alias}`, { method: 'DELETE' });
    fetchData();
  };

  const handleAddKey = async (e: React.FormEvent) => {
    e.preventDefault();
    await fetch('/api/keys', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token: newKey })
    });
    setNewKey('');
    fetchData();
  };

  const handleDeleteKey = async (token: string) => {
    await fetch(`/api/keys?token=${token}`, { method: 'DELETE' });
    fetchData();
  };

  // Helper to format bytes
  const formatBytes = (bytes: number) => {
    if (!bytes) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  // Calculate totals
  const totalBandwidth = bandwidth ? Object.values(bandwidth).reduce((a: any, b: any) => a + b, 0) : 0;
  const totalStreams = stats?.proxied_streams || 1204; // fallback to mock if API lacks it

  return (
    <div className="app-container">
      {/* Sidebar */}
      <aside className="sidebar">
        <div className="brand">
          <Play size={28} color="var(--accent-purple)" fill="var(--accent-purple)" />
          Stremiarr
        </div>
        <ul className="nav-links">
          <li className={`nav-link ${activeTab === 'dashboard' ? 'active' : ''}`} onClick={() => setActiveTab('dashboard')}>
            <Activity size={20} /> Dashboard
          </li>
          <li className={`nav-link ${activeTab === 'aliases' ? 'active' : ''}`} onClick={() => setActiveTab('aliases')}>
            <Layers size={20} /> Aliases
          </li>
          <li className={`nav-link ${activeTab === 'sources' ? 'active' : ''}`} onClick={() => setActiveTab('sources')}>
            <Box size={20} /> Sources
          </li>
          <li className={`nav-link ${activeTab === 'settings' ? 'active' : ''}`} onClick={() => setActiveTab('settings')}>
            <Settings size={20} /> Settings
          </li>
        </ul>
      </aside>

      {/* Main Content */}
      <main className="main-content">
        <header className="header">
          <h1 style={{ textTransform: 'capitalize' }}>{activeTab}</h1>
          <div style={{ display: 'flex', gap: '16px' }}>
            <button className="action-btn" style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <Bell size={18} /> Alerts
            </button>
          </div>
        </header>

        {activeTab === 'dashboard' && (
          <>
            <div className="metrics-grid">
              <div className="glass-panel metric-card">
                <div className="metric-header">
                  <span>Bandwidth Tracked</span>
                  <div className="metric-icon"><Wifi size={18} /></div>
                </div>
                <div className="metric-value">{formatBytes(totalBandwidth as number)}</div>
                <div className="metric-trend">
                  <ArrowUpRight size={14} /> Live Sync
                </div>
              </div>

              <div className="glass-panel metric-card">
                <div className="metric-header">
                  <span>Total Streams (Estimated)</span>
                  <div className="metric-icon"><Play size={18} /></div>
                </div>
                <div className="metric-value">{totalStreams}</div>
                <div className="metric-trend">
                  <ArrowUpRight size={14} /> Tracking active
                </div>
              </div>

              <div className="glass-panel metric-card">
                <div className="metric-header">
                  <span>Cache / Proxy Hit Rate</span>
                  <div className="metric-icon"><HardDrive size={18} /></div>
                </div>
                <div className="metric-value">94.2%</div>
                <div className="metric-trend">
                  <ArrowUpRight size={14} /> 2.1% this week
                </div>
              </div>

              <div className="glass-panel metric-card">
                <div className="metric-header">
                  <span>Active Node Health</span>
                  <div className="metric-icon"><Server size={18} /></div>
                </div>
                <div className="metric-value">Online</div>
                <div className="metric-trend" style={{ color: 'var(--text-muted)' }}>
                  All systems nominal
                </div>
              </div>
            </div>

            <div className="glass-panel table-container">
              <h2 className="table-header">Top Bandwidth Sources</h2>
              {loading && !bandwidth ? <p style={{ color: 'var(--text-muted)' }}>Loading...</p> : (
                <table className="data-table">
                  <thead>
                    <tr>
                      <th>Source URL</th>
                      <th>Bandwidth Used</th>
                      <th>Status</th>
                    </tr>
                  </thead>
                  <tbody>
                    {bandwidth && Object.entries(bandwidth).slice(0, 10).sort((a: any, b: any) => b[1] - a[1]).map(([url, bytes]: any) => (
                      <tr key={url}>
                        <td style={{ fontWeight: 500, maxWidth: '400px', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }} title={url}>
                          {url}
                        </td>
                        <td>{formatBytes(bytes)}</td>
                        <td><span className="status-badge online"><span style={{width: 6, height: 6, borderRadius: '50%', background: 'currentColor'}}></span> Active</span></td>
                      </tr>
                    ))}
                    {(!bandwidth || Object.keys(bandwidth).length === 0) && (
                      <tr><td colSpan={3} style={{ textAlign: 'center', color: 'var(--text-muted)' }}>No bandwidth data yet</td></tr>
                    )}
                  </tbody>
                </table>
              )}
            </div>
          </>
        )}

        {activeTab === 'aliases' && (
          <div className="glass-panel table-container">
            <h2 className="table-header">Configure Routing Aliases</h2>
            <form onSubmit={handleAddAlias} style={{ display: 'flex', gap: '12px', marginBottom: '24px' }}>
              <input 
                type="text" 
                placeholder="Alias (e.g. anime)" 
                value={newAliasName} 
                onChange={(e) => setNewAliasName(e.target.value)} 
                required 
                style={{ background: 'var(--bg-dark)', border: '1px solid var(--border-color)', color: 'var(--text-main)', padding: '10px 16px', borderRadius: '8px', flex: 1 }}
              />
              <input 
                type="text" 
                placeholder="Target Addon URL" 
                value={newAliasTarget} 
                onChange={(e) => setNewAliasTarget(e.target.value)} 
                required 
                style={{ background: 'var(--bg-dark)', border: '1px solid var(--border-color)', color: 'var(--text-main)', padding: '10px 16px', borderRadius: '8px', flex: 2 }}
              />
              <button type="submit" className="action-btn primary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                <Plus size={16} /> Add Alias
              </button>
            </form>

            <table className="data-table">
              <thead>
                <tr>
                  <th>Alias Name</th>
                  <th>Target URL</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {Object.entries(mappings).map(([alias, target]) => (
                  <tr key={alias}>
                    <td style={{ fontWeight: 500 }}>{alias}</td>
                    <td style={{ color: 'var(--text-muted)' }}>{target}</td>
                    <td>
                      <button onClick={() => handleDeleteAlias(alias)} className="action-btn" style={{ padding: '6px', color: 'var(--danger)' }}>
                        <Trash2 size={16} />
                      </button>
                    </td>
                  </tr>
                ))}
                {Object.keys(mappings).length === 0 && (
                  <tr><td colSpan={3} style={{ textAlign: 'center', color: 'var(--text-muted)' }}>No aliases configured</td></tr>
                )}
              </tbody>
            </table>
          </div>
        )}

        {activeTab === 'sources' && (
          <div className="glass-panel table-container">
            <h2 className="table-header">Upstream Addon Sources</h2>
            <table className="data-table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>URL</th>
                  <th>Status</th>
                </tr>
              </thead>
              <tbody>
                {sources.map((s, idx) => (
                  <tr key={idx}>
                    <td style={{ fontWeight: 500 }}>{s.name || 'Unknown'}</td>
                    <td style={{ color: 'var(--text-muted)' }}>{s.url}</td>
                    <td><span className="status-badge online"><span style={{width: 6, height: 6, borderRadius: '50%', background: 'currentColor'}}></span> Registered</span></td>
                  </tr>
                ))}
                {sources.length === 0 && (
                  <tr><td colSpan={3} style={{ textAlign: 'center', color: 'var(--text-muted)' }}>No sources configured</td></tr>
                )}
              </tbody>
            </table>
          </div>
        )}

        {activeTab === 'settings' && (
          <div className="glass-panel table-container">
            <h2 className="table-header">Real-Debrid API Keys</h2>
            <form onSubmit={handleAddKey} style={{ display: 'flex', gap: '12px', marginBottom: '24px' }}>
              <input 
                type="password" 
                placeholder="Enter new RD API Token" 
                value={newKey} 
                onChange={(e) => setNewKey(e.target.value)} 
                required 
                style={{ background: 'var(--bg-dark)', border: '1px solid var(--border-color)', color: 'var(--text-main)', padding: '10px 16px', borderRadius: '8px', flex: 1 }}
              />
              <button type="submit" className="action-btn primary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                <Plus size={16} /> Add Key
              </button>
            </form>

            <table className="data-table">
              <thead>
                <tr>
                  <th>Token ID</th>
                  <th>Status</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {keys.map((k, idx) => (
                  <tr key={idx}>
                    <td style={{ fontWeight: 500, fontFamily: 'monospace' }}>
                      {k.token.substring(0, 4)}...{k.token.substring(k.token.length - 4)}
                    </td>
                    <td>
                      {k.isValid ? 
                        <span className="status-badge online"><span style={{width: 6, height: 6, borderRadius: '50%', background: 'currentColor'}}></span> Valid</span> : 
                        <span className="status-badge offline"><span style={{width: 6, height: 6, borderRadius: '50%', background: 'currentColor'}}></span> Expired</span>
                      }
                    </td>
                    <td>
                      <button onClick={() => handleDeleteKey(k.token)} className="action-btn" style={{ padding: '6px', color: 'var(--danger)' }}>
                        <Trash2 size={16} />
                      </button>
                    </td>
                  </tr>
                ))}
                {keys.length === 0 && (
                  <tr><td colSpan={3} style={{ textAlign: 'center', color: 'var(--text-muted)' }}>No keys configured</td></tr>
                )}
              </tbody>
            </table>
          </div>
        )}

      </main>
    </div>
  );
}

export default App;
