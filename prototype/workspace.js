// GUI workspaces contain views of core resources, never independent SSH clients.
let workspaceSequence = 0;
const workspaceId = prefix => `${prefix}-${Date.now()}-${++workspaceSequence}`;
const paneNode = (id, session, shell) => ({type:'pane',id,session,shell,input:false,minimized:false});
const initialSSHState = () => ({
  active:'w-demo-01',hostQuery:'',hostGroup:'全部',
  workspaces:[
    {id:'w-demo-01',name:'开发与预发布',mode:'split',focus:'p-demo-01',maximized:null,tree:{type:'split',id:'split-demo-01',axis:'row',ratio:55,first:paneNode('p-demo-01','s-demo-01','sh-demo-01'),second:paneNode('p-demo-02','s-demo-03','sh-demo-04')}},
    {id:'w-demo-02',name:'本地工作区',mode:'split',focus:'p-demo-03',maximized:null,tree:paneNode('p-demo-03','s-demo-02','sh-demo-03')}
  ]
});
function workspace(){return state.ssh.workspaces.find(w=>w.id===state.ssh.active);}
function leaves(node){return !node?[]:node.type==='pane'?[node]:[...leaves(node.first),...leaves(node.second)];}
function focusedPane(){const w=workspace();return w&&leaves(w.tree).find(p=>p.id===w.focus);}
function findSplit(node,id){if(!node||node.type==='pane')return null;return node.id===id?node:findSplit(node.first,id)||findSplit(node.second,id);}
function filterTree(node,predicate){
  if(!node)return null;
  if(node.type==='pane')return predicate(node)?node:null;
  const first=filterTree(node.first,predicate),second=filterTree(node.second,predicate);
  return first&&second?{...node,first,second}:first||second;
}
function replacePane(node,id,replacement){
  if(!node)return null;
  if(node.type==='pane')return node.id===id?replacement:node;
  return {...node,first:replacePane(node.first,id,replacement),second:replacePane(node.second,id,replacement)};
}
function reconcileWorkspaces(){
  if(!state.ssh)state.ssh=initialSSHState();
  for(const w of state.ssh.workspaces){
    w.tree=filterTree(w.tree,p=>state.sessions.some(s=>s.id===p.session&&s.shells.some(sh=>sh.id===p.shell)));
    const panes=leaves(w.tree);
    if(!panes.some(p=>p.id===w.focus&&!p.minimized))w.focus=panes.find(p=>!p.minimized)?.id;
    if(!panes.some(p=>p.id===w.maximized&&!p.minimized))w.maximized=null;
  }
  if(state.page==='workspace'&&available())syncFocus();
}
function syncFocus(){const p=focusedPane();if(p){state.session=p.session;state.shell=p.shell;state.input=p.input;}else{state.session='';state.shell='';state.input=false;}}
function focusPane(id){const w=workspace();if(!w||!leaves(w.tree).some(p=>p.id===id))return;w.focus=id;syncFocus();}
function openSessionView(session,shellId){
  if(!available())return;
  let shell=session.shells.find(sh=>sh.id===shellId)||session.shells[0];
  if(!shell){shell={id:workspaceId('sh-demo'),name:'bash',exited:false};session.shells.push(shell);toast('已在当前会话上新建 Shell（模拟），没有新建连接。');}
  for(const w of state.ssh.workspaces){const p=leaves(w.tree).find(p=>p.session===session.id&&p.shell===shell.id);if(p){state.ssh.active=w.id;p.minimized=false;w.maximized=null;w.focus=p.id;state.page='workspace';syncFocus();return;}}
  const p=paneNode(workspaceId('pane'),session.id,shell.id);
  const w={id:workspaceId('workspace'),name:session.name,mode:'split',focus:p.id,maximized:null,tree:p};
  state.ssh.workspaces.push(w);state.ssh.active=w.id;state.page='workspace';syncFocus();
}
function addPane(p,axis='row'){
  const w=workspace();if(!w)return;
  if(leaves(w.tree).length>=8){toast('原型每个工作区最多展示 8 个终端，请新建工作区。');return false;}
  const target=focusedPane()||leaves(w.tree)[0];
  w.tree=target?replacePane(w.tree,target.id,{type:'split',id:workspaceId('split'),axis,ratio:50,first:target,second:p}):p;
  w.focus=p.id;w.maximized=null;syncFocus();return true;
}
function newCoreSession(profile,name,command='bash'){
  const session={id:workspaceId('s-demo'),name:name||profile.name,connection:profile.name,host:profile.kind==='internal'?'本机':profile.host,kind:profile.kind,shells:[{id:workspaceId('sh-demo'),name:command,exited:false}],waiting:false};
  state.sessions.push(session);event('SSH 会话已创建',`${profile.name} · termcp · 演示`);return session;
}
function hostGroups(){return ['全部','收藏',...new Set(state.connections.map(c=>c.group|| (c.kind==='internal'?'本机':'未分组')))];}
function hostMatches(c){return (state.ssh.hostGroup==='全部'||state.ssh.hostGroup==='收藏'&&c.favorite||state.ssh.hostGroup===(c.group||'未分组'))&&(c.name+' '+c.host+' '+c.user).toLowerCase().includes(state.ssh.hostQuery.toLowerCase());}
function hostNavigation(){
  return `<div class="host-library"><div class="host-library-head"><span>HOSTS</span>${iconButton('添加 SSH 主机','new-connection','add',!available()?'disabled':'')}</div>${[...new Set(state.connections.map(c=>c.group|| (c.kind==='internal'?'本机':'未分组')))].map(group=>`<div class="host-nav-group"><span>${escapeHtml(group)}</span>${state.connections.filter(c=>(c.group||(c.kind==='internal'?'本机':'未分组'))===group).map(c=>`<button class="host-nav-item ${current()?.connection===c.name&&state.page==='workspace'?'selected':''}" data-action="host-connect" data-id="${escapeHtml(c.name)}" ${!available()?'disabled':''}><span class="host-glyph">${c.kind==='internal'?'›_':'▣'}</span><span>${escapeHtml(c.name)}<small>${escapeHtml(c.kind==='internal'?'本机':c.host)}</small></span>${c.favorite?'<span class="host-star">★</span>':''}</button>`).join('')}</div>`).join('')}</div>`;
}
function sshWorkspacePage(){
  if(!available())return head('SSH 工作台','由 termcp 管理连接，GUI 组织主机与终端布局。')+runtime()+'<div class="empty"><h3>当前 Core 不可用</h3><p>启动或连接 core 后，即可连接主机并创建终端工作区。</p>'+button('管理 Core','go-core','primary')+'</div>';
  const w=workspace(),p=focusedPane(),s=current(),sh=s?.shells.find(sh=>sh.id===state.shell);
  const visible=leaves(w?.tree).filter(p=>!p.minimized),minimized=leaves(w?.tree).filter(p=>p.minimized);
  const tabBar=`<nav class="workspace-tabs" aria-label="终端工作区">${state.ssh.workspaces.map(w=>`<div class="workspace-tab ${w.id===state.ssh.active?'active':''}"><button data-action="activate-workspace" data-id="${w.id}" aria-pressed="${w.id===state.ssh.active}">${escapeHtml(w.name)}<span>${leaves(w.tree).length}</span></button>${iconButton('关闭工作区视图 · 保留连接','close-workspace','close',`data-id="${w.id}"`)}</div>`).join('')}${iconButton('新建空工作区','new-workspace','add')}</nav>`;
  const toolbar=`<div class="workspace-toolbar"><div class="workspace-runtime"><span class="dot"></span>Local core <small>:${escapeHtml(state.config.port)}</small>${iconButton('管理 Core','go-core','settings')}</div><div class="layout-tools"><span class="toolbar-label">布局</span>${iconButton('左右分屏 · 新建同连接 Shell','split-horizontal','splitH',!p?'disabled':'')}${iconButton('上下分屏 · 新建同连接 Shell','split-vertical','splitV',!p?'disabled':'')}${iconButton('平铺全部终端','tile-panes','grid',`aria-pressed="${w?.mode==='tile'}" ${!visible.length?'disabled':''}`)}${iconButton('恢复分屏布局','split-mode','splitH',`aria-pressed="${w?.mode==='split'}" ${!visible.length?'disabled':''}`)}<span class="toolbar-divider"></span>${iconButton('添加现有 Shell 或其他主机','add-terminal','add')}${iconButton('重命名工作区','rename-workspace','edit')}${iconButton(state.inspectorCollapsed?'展开主机详情':'收起主机详情','toggle-inspector','panel',`aria-expanded="${!state.inspectorCollapsed}"`)}</div></div>`;
  let canvas='';
  if(!visible.length)canvas='<div class="workspace-empty"><span>›_</span><h3>从一台主机开始</h3><p>连接主机，或把 core 中已有的 Shell 加入工作区。</p>'+button('添加终端','add-terminal','primary')+'</div>';
  else if(w.maximized)canvas=paneWindow(leaves(w.tree).find(p=>p.id===w.maximized));
  else if(w.mode==='tile')canvas=`<div class="tile-layout">${visible.map(paneWindow).join('')}</div>`;
  else canvas=splitTree(filterTree(w.tree,p=>!p.minimized));
  return head('SSH 工作台','主机连接、终端分屏和会话资源，在同一个窗口。',button('＋ 连接主机','connect-picker','primary'))+tabBar+toolbar+
    `<div class="ssh-workspace ${state.inspectorCollapsed?'inspector-collapsed':''}"><div class="workspace-center"><div class="terminal-stage" data-mode="${w?.mode||'split'}">${canvas}</div>${minimized.length?`<div class="minimized-dock"><span>已收起</span>${minimized.map(p=>{const s=state.sessions.find(s=>s.id===p.session),sh=s.shells.find(sh=>sh.id===p.shell);return button(`${escapeHtml(s.connection)} / ${escapeHtml(sh.name)} ↗`,'restore-pane','',`data-id="${p.id}"`);}).join('')}</div>`:''}<div class="workspace-context-tools"><nav class="tool-tabs" aria-label="当前终端资源">${[['files','文件'],['forwards','转发'],['rules','通知']].map(([key,name])=>`<button class="${state.tool===key?'active':''}" data-tool="${key}" aria-pressed="${state.tool===key}">${name}${key==='forwards'?` · ${currentForwards().length}`:''}</button>`).join('')}<span class="context-host">${s?`${escapeHtml(s.connection)} / ${escapeHtml(sh?.name||'')}`:'未选择终端'}</span></nav><div class="tool-content">${s?toolContent(s):'<div class="note">选择终端后查看文件、转发和通知。</div>'}</div></div></div>${s?inspector(s,sh):'<aside class="workspace-info inspector"><p class="note">选择一个终端查看会话信息。</p></aside>'}</div>`+
    `<div class="workspace-status"><span>${state.sessions.length} 个会话 · ${allShells()} 个运行中 Shell</span><span>焦点输入 · termcp 共享会话</span><span>${w?.mode==='tile'?'平铺':'分屏'} / ${visible.length} 个窗格</span></div>`;
}
function splitTree(node){
  if(!node)return '';
  if(node.type==='pane')return paneWindow(node);
  return `<div class="split-node axis-${node.axis}" data-split="${node.id}"><div class="split-child" style="flex:${node.ratio}">${splitTree(node.first)}</div><div class="splitter" role="separator" tabindex="0" aria-label="调整分屏比例" aria-orientation="${node.axis==='row'?'vertical':'horizontal'}" aria-valuemin="20" aria-valuemax="80" aria-valuenow="${node.ratio}" data-divider="${node.id}"></div><div class="split-child" style="flex:${100-node.ratio}">${splitTree(node.second)}</div></div>`;
}
function paneOutput(s,sh){
  const c=state.connections.find(c=>c.name===s.connection),user=escapeHtml(c?.user==='—'?'workspace':c?.user||'developer');
  if(sh.name==='logs')return '<span class="dim">Application logs · live</span>\n\n<span class="green">INFO</span>  Listening on :3000\n<span class="green">INFO</span>  GET /health 200 2ms\n<span class="green">INFO</span>  Database connection ready\n<span class="dim">Watching for changes…</span>';
  if(sh.name==='monitor')return '<span class="dim">staging / service status</span>\n\n<span class="green">●</span> api-service   running\n<span class="green">●</span> worker        running\n<span class="green">●</span> postgres      running\n\n<span class="dim">$ journalctl -f -u api-service</span>\n<span class="green">INFO</span> Health check passed\n<span class="green">INFO</span> Request completed 200';
  if(sh.name==='build')return `<span class="dim">${escapeHtml(s.host)} · SSH · PTY</span>\n\n<span class="green">${user}@dev ~/workspace $</span> npm run build\n\n<span class="green">✓</span> Type check completed\n<span class="green">✓</span> Build completed\n\n${s.waiting?'<span class="yellow">[sudo] password for developer:</span>':''}`;
  return `<span class="dim">Connected · ${escapeHtml(s.host)} · ${s.kind==='internal'?'local':'SSH'} · PTY</span>\n\n<span class="green">${user}@${s.kind==='internal'?'local':escapeHtml(s.connection)} ~/workspace $</span> `;
}
function paneWindow(p){
  if(!p)return '';
  const s=state.sessions.find(s=>s.id===p.session),sh=s?.shells.find(sh=>sh.id===p.shell);if(!s||!sh)return '';
  const w=workspace();
  return `<section class="terminal pane-window ${w.focus===p.id?'pane-focused':''}" data-pane="${p.id}" aria-label="${escapeHtml(s.connection)} / ${escapeHtml(sh.name)}"><div class="pane-titlebar"><button class="pane-title" data-action="focus-pane" data-id="${p.id}" aria-pressed="${w.focus===p.id}"><span class="dot ${sh.exited?'gray':s.waiting?'amber':''}"></span><b>${escapeHtml(s.connection)}</b><span>/ ${escapeHtml(sh.name)}</span></button><div class="window-controls">${iconButton('管理此会话','manage-session','settings')}${iconButton('收起此终端 · 保留 Shell','minimize-pane','minimize',`data-id="${p.id}"`)}${iconButton(w.maximized===p.id?'还原终端布局':'最大化此终端','maximize-pane',w.maximized===p.id?'restore':'expand',`data-id="${p.id}"`)}${iconButton('移除此窗格 · 保留 Shell','remove-pane','close',`data-id="${p.id}"`)}</div></div><pre class="terminal-output pane-output">${paneOutput(s,sh)}${state.output[p.shell]||''}${sh.exited?'\n<span class="dim">Shell 已退出 · 只读输出</span>':''}</pre><form class="terminal-input pane-input" data-pane-form="${p.id}"><button type="button" class="input-mode ${p.input?'enabled':''}" data-action="pane-input-mode" data-id="${p.id}" aria-pressed="${p.input}" ${sh.exited?'disabled':''}>${sh.exited?'只读':p.input?'输入':'观察'}</button>${iconButton("中断前台程序 · Ctrl+C","interrupt","interrupt",`type="button" ${sh.exited?"disabled":""}`)}<label class="sr-only" for="command-${p.id}">输入到 ${escapeHtml(s.connection)} / ${escapeHtml(sh.name)}</label><input id="command-${p.id}" name="command" placeholder="${p.input?'输入演示命令':'开启输入后操作终端'}" ${!p.input||sh.exited?'disabled':''} autocomplete="off"><button type="submit" aria-label="发送到 ${escapeHtml(s.connection)} / ${escapeHtml(sh.name)}" ${!p.input||sh.exited?'disabled':''}>↵</button></form></section>`;
}
function connectPicker(target='tab'){
  if(!available()){toast('请先启动或连接 core。');return;}
  openDialog(target==='split'?'将主机连接加入分屏':'连接 SSH 主机',`<form id="host-connect-form"><label class="field">主机配置<select name="profile">${state.connections.map(c=>`<option value="${escapeHtml(c.name)}">${escapeHtml(c.name)} · ${escapeHtml(c.user)}@${escapeHtml(c.host)}</option>`).join('')}</select></label><label class="field">会话名称<input name="name" placeholder="留空使用主机名称" maxlength="60"></label><label class="field">启动 Shell<input name="command" value="bash" required maxlength="60"></label><label class="field">打开位置<select name="target"><option value="tab" ${target==='tab'?'selected':''}>新工作区标签</option><option value="split" ${target==='split'?'selected':''}>当前工作区分屏</option></select></label><div class="mode-note">由当前 core 建立一条新连接。其他终端和连接保持运行。</div></form>`,button('取消','close-dialog')+button('连接并打开','confirm-host-connect','primary'));
}
function profileEditor(name){
  if(!available())return;
  const c=state.connections.find(c=>c.name===name);if(c?.kind==='internal')return;
  openDialog(c?'编辑 SSH 主机':'添加 SSH 主机',`<form id="ssh-profile-form" data-original="${escapeHtml(name||'')}"><div class="profile-form-grid"><label class="field">配置名称<input name="name" value="${escapeHtml(c?.name||'')}" placeholder="my-server" pattern="[A-Za-z0-9_-]+" ${c?'readonly':''} required></label><label class="field">分组<input name="group" value="${escapeHtml(c?.group||'开发')}" maxlength="30" required></label><label class="field">主机<input name="host" value="${escapeHtml(c?.host||'')}" placeholder="server.example.test" required></label><label class="field">端口<input name="port" type="number" min="1" max="65535" value="${c?.port||22}" required></label><label class="field">用户<input name="user" value="${escapeHtml(c?.user||'')}" placeholder="developer" required></label><label class="field">认证<select name="auth">${['密码','私钥 PEM 内容'].map(v=>`<option ${c?.auth===v?'selected':''}>${v}</option>`).join('')}</select></label></div><label class="field">凭据<input name="credential" type="password" placeholder="演示字段，不保存凭据" autocomplete="new-password"><small>已存凭据不可读回。正式版私钥需提供完整 PEM 内容。</small></label><label class="field">跳板机配置（可选）<select name="jump"><option value="">直接连接</option>${state.connections.filter(x=>x.kind==='remote'&&x.name!==c?.name).map(x=>`<option value="${escapeHtml(x.name)}" ${c?.jump===x.name?'selected':''}>${escapeHtml(x.name)}</option>`).join('')}</select></label><label class="checkbox"><input name="favorite" type="checkbox" ${c?.favorite?'checked':''}>收藏此主机</label><div id="ssh-profile-error" role="alert" class="error-text"></div></form>`,button('取消','close-dialog')+button('测试配置','test-profile')+button('保存主机','save-profile','primary'));
}
function sshHostsPage(){
  if(!available())return head('主机管理','连接配置保存在当前 core，分组与收藏由 GUI 组织。')+runtime()+'<div class="empty">当前 Core 不可用。</div>';
  return head('主机管理','选择主机建立会话，再把终端放入标签、分屏或平铺工作区。',button('＋ 添加主机','new-connection','primary'))+runtime()+`<div class="host-filter"><input type="search" class="host-search" aria-label="搜索 SSH 主机" placeholder="搜索名称、主机或用户" value="${escapeHtml(state.ssh.hostQuery)}" data-host-filter><div class="host-groups">${hostGroups().map(g=>`<button data-action="host-group" data-id="${escapeHtml(g)}" aria-pressed="${state.ssh.hostGroup===g}">${escapeHtml(g)}</button>`).join('')}</div></div><div id="host-results">${hostResults()}</div>`;
}
function hostResults(){
  const hosts=state.connections.filter(hostMatches);
  if(!hosts.length)return '<div class="empty">没有匹配的主机。</div>';
  return `<div class="host-cards">${hosts.map(c=>{const sessions=state.sessions.filter(s=>s.connection===c.name);return `<article class="host-card"><div class="host-card-top"><span class="session-symbol">${c.kind==='internal'?'›_':'⌁'}</span><div><h3>${escapeHtml(c.name)}</h3><small>${escapeHtml(c.group||'未分组')} · ${c.kind==='internal'?'本机':escapeHtml(c.user)+'@'+escapeHtml(c.host)+':'+(c.port||22)}</small></div>${iconButton(c.favorite?'取消收藏':'收藏主机','favorite-host','star',`data-id="${escapeHtml(c.name)}" aria-pressed="${!!c.favorite}"`)}</div><div class="host-card-meta"><span>${sessions.length?`<span class="dot"></span>${sessions.length} 条会话运行中`:'未建立连接'}</span><span>${c.jump?'经 '+escapeHtml(c.jump):c.kind==='internal'?'内建连接':c.auth||'私钥'}</span></div><div class="host-card-actions">${button(c.kind==='internal'?'打开本机终端':'连接主机','host-connect','primary',`data-id="${escapeHtml(c.name)}"`)}${iconButton('连接到当前分屏','host-connect-split','splitH',`data-id="${escapeHtml(c.name)}"`)}${iconButton('主机详情与管理','connection-detail','settings',`data-id="${escapeHtml(c.name)}"`)}</div></article>`;}).join('')}</div>`;
}
function hostDetail(name){
  const c=state.connections.find(c=>c.name===name);if(!c)return;
  openDialog('主机 · '+escapeHtml(c.name),`<div class="list-line"><span>所属 Core</span><span>Local core</span></div><div class="list-line"><span>连接地址</span><code>${escapeHtml(c.kind==='internal'?'本机':c.user+'@'+c.host+':'+(c.port||22))}</code></div><div class="list-line"><span>分组 / 跳板</span><span>${escapeHtml(c.group||'未分组')} / ${escapeHtml(c.jump||'无')}</span></div><div class="list-line"><span>凭据</span><span>${c.kind==='internal'?'内建连接':'不可读回 · 演示'}</span></div><div class="section-head"><h3>运行中的会话</h3></div>${state.sessions.filter(s=>s.connection===c.name).map(s=>`<div class="list-line"><span>${escapeHtml(s.name)}</span>${button('打开','select-session','',`data-id="${s.id}"`)}</div>`).join('')||'<p class="note">尚未建立会话。</p>'}`,button('关闭','close-dialog')+(c.kind==='remote'?button('删除配置','delete-profile','danger',`data-id="${escapeHtml(c.name)}"`)+button('编辑','edit-profile','',`data-id="${escapeHtml(c.name)}"`):'')+button('连接','host-connect','primary',`data-id="${escapeHtml(c.name)}"`));
}
function splitShell(axis='row',name='bash'){
  const s=current(),w=workspace();if(!available()||!s||!w)return;
  if(leaves(w.tree).length>=8){toast('请新建工作区，当前最多展示 8 个终端。');return;}
  const sh={id:workspaceId('sh-demo'),name:name==='bash'?`shell-${s.shells.length+1}`:name,exited:false};s.shells.push(sh);
  addPane(paneNode(workspaceId('pane'),s.id,sh.id),axis);w.mode='split';render();toast('已在当前连接上新建 Shell 分屏（模拟）');
}
const workspaceActions={
  'inspector-tab':b=>{state.inspectorTab=b.dataset.id;render();},
  'inspect-shell':b=>{const s=state.sessions.find(s=>s.shells.some(sh=>sh.id===b.dataset.id));if(s){openSessionView(s,b.dataset.id);render();}},
  'focus-pane':b=>{focusPane(b.dataset.id);render();},
  'activate-workspace':b=>{state.ssh.active=b.dataset.id;state.page='workspace';syncFocus();render();},
  'new-workspace':()=>{const w={id:workspaceId('workspace'),name:`工作区 ${state.ssh.workspaces.length+1}`,mode:'split',tree:null,focus:null,maximized:null};state.ssh.workspaces.push(w);state.ssh.active=w.id;state.page='workspace';render();},
  'close-workspace':b=>{state.ssh.workspaces=state.ssh.workspaces.filter(w=>w.id!==b.dataset.id);if(state.ssh.active===b.dataset.id)state.ssh.active=state.ssh.workspaces[0]?.id;if(!workspace())workspaceActions['new-workspace']();else render();toast('工作区视图已关闭，Core 会话保持运行');},
  'rename-workspace':()=>openDialog('重命名工作区',`<form id="workspace-name-form"><label class="field">名称<input name="name" value="${escapeHtml(workspace()?.name||'')}" required maxlength="40"></label></form>`,button('取消','close-dialog')+button('保存','save-workspace-name','primary')),
  'save-workspace-name':()=>{const f=$('#workspace-name-form');if(!f.reportValidity())return;workspace().name=new FormData(f).get('name').trim();closeDialog();render();},
  'split-horizontal':()=>splitShell('row'),'split-vertical':()=>splitShell('column'),
  'tile-panes':()=>{workspace().mode='tile';workspace().maximized=null;render();},
  'split-mode':()=>{workspace().mode='split';workspace().maximized=null;render();},
  'minimize-pane':b=>{const p=leaves(workspace().tree).find(p=>p.id===b.dataset.id);p.minimized=true;workspace().maximized=null;render();},
  'restore-pane':b=>{const p=leaves(workspace().tree).find(p=>p.id===b.dataset.id);p.minimized=false;focusPane(p.id);render();},
  'maximize-pane':b=>{const w=workspace();w.maximized=w.maximized===b.dataset.id?null:b.dataset.id;render();},
  'remove-pane':b=>{const w=workspace();w.tree=filterTree(w.tree,p=>p.id!==b.dataset.id);render();toast('窗格已移除，Shell 继续运行；可通过添加终端重新打开');},
  'pane-input-mode':b=>{const p=focusedPane();p.input=!p.input;render();if(p.input)document.getElementById('command-'+p.id)?.focus();},
  'enable-input':()=>{const p=focusedPane();if(!p)return;p.input=true;p.minimized=false;render();document.getElementById('command-'+p.id)?.focus();},
  'disable-input':()=>{const p=focusedPane();if(p)p.input=false;render();},
  'connect-picker':()=>connectPicker(),
  'host-connect':b=>{if(!available())return;const c=state.connections.find(c=>c.name===b.dataset.id);if(!c)return;closeDialog();openSessionView(newCoreSession(c));render();toast('Core 已建立新会话（模拟）');},
  'host-connect-split':b=>{if(!available())return;const c=state.connections.find(c=>c.name===b.dataset.id);if(!c)return;if(!workspace()){openSessionView(newCoreSession(c));}else{if(leaves(workspace().tree).length>=8){toast('当前工作区已满，请新建工作区。');return;}const s=newCoreSession(c);addPane(paneNode(workspaceId('pane'),s.id,s.shells[0].id));}state.page='workspace';closeDialog();render();toast('新主机会话已加入工作区（模拟）');},
  'confirm-host-connect':()=>{const f=$('#host-connect-form');if(!f.reportValidity())return;const d=new FormData(f),c=state.connections.find(c=>c.name===d.get('profile'));if(d.get('target')==='split'&&leaves(workspace()?.tree).length>=8){toast('当前工作区已满，请选择新标签。');return;}const s=newCoreSession(c,d.get('name').trim(),d.get('command').trim());if(d.get('target')==='split'&&workspace()){addPane(paneNode(workspaceId('pane'),s.id,s.shells[0].id));state.page='workspace';}else openSessionView(s);closeDialog();render();toast('新连接已由 Core 创建（模拟）');},
  'add-terminal':()=>{if(!available())return;const w=workspace();if(leaves(w?.tree).length>=8){toast('当前工作区已满，请新建工作区。');return;}openDialog('添加终端',`<p class="note">选择已有 Shell 不会新建连接，也不会重启进程。</p><div class="existing-shells">${state.sessions.map(s=>s.shells.map(sh=>`<button data-action="add-existing-shell" data-session="${s.id}" data-shell-ref="${sh.id}"><span>${escapeHtml(s.connection)} / ${escapeHtml(sh.name)}</span>${badge(sh.exited?'只读':'运行中',sh.exited?'neutral':'')}</button>`).join('')).join('')||'<p class="note">没有可用 Shell。</p>'}</div>`,button('取消','close-dialog')+button('连接另一台主机','connect-other-host','primary'));},
  'connect-other-host':()=>connectPicker('split'),
  'add-existing-shell':b=>{const w=workspace(),existing=leaves(w.tree).find(p=>p.session===b.dataset.session&&p.shell===b.dataset.shellRef);if(existing){existing.minimized=false;w.maximized=null;focusPane(existing.id);}else addPane(paneNode(workspaceId('pane'),b.dataset.session,b.dataset.shellRef));closeDialog();render();},
  'select-session':b=>{const s=state.sessions.find(s=>s.id===b.dataset.id);if(s){closeDialog();openSessionView(s);render();}},
  'waiting':()=>{const s=state.sessions.find(s=>s.waiting);if(s){openSessionView(s);render();}},
  'create-session':()=>{const f=$('#new-session-form');if(!f.reportValidity())return;const d=new FormData(f),c=state.connections.find(c=>c.name===d.get('connection'));openSessionView(newCoreSession(c,d.get('name').trim(),d.get('command').trim()));closeDialog();render();},
  'create-shell':()=>{const f=$('#shell-form');if(!f.reportValidity())return;const name=new FormData(f).get('name').trim();closeDialog();splitShell('row',name);},
  'new-connection':()=>profileEditor(),
  'connection-detail':b=>hostDetail(b.dataset.id),
  'edit-profile':b=>profileEditor(b.dataset.id),
  'test-profile':()=>{if($('#ssh-profile-form').reportValidity())toast('SSH 测试通过（模拟）；没有发起真实连接');},
  'save-profile':()=>{const f=$('#ssh-profile-form');if(!f.reportValidity())return;const d=new FormData(f),original=f.dataset.original;if(d.get('jump')){let parent=d.get('jump');const seen=new Set([d.get('name')]);while(parent){if(seen.has(parent)){$('#ssh-profile-error').textContent='跳板机配置不能形成循环。';return;}seen.add(parent);parent=state.connections.find(c=>c.name===parent)?.jump;}}if(state.connections.some(c=>c.name===d.get('name')&&c.name!==original)){$('#ssh-profile-error').textContent='配置名称已存在。';return;}const c={name:d.get('name'),group:d.get('group').trim(),host:d.get('host').trim(),port:Number(d.get('port')),user:d.get('user').trim(),auth:d.get('auth'),jump:d.get('jump'),favorite:d.has('favorite'),kind:'remote'};const index=state.connections.findIndex(c=>c.name===original);if(index>=0)state.connections[index]=c;else state.connections.push(c);closeDialog();render();toast('主机配置已保存（模拟）；凭据不保存');},
  'favorite-host':b=>{const c=state.connections.find(c=>c.name===b.dataset.id);c.favorite=!c.favorite;render();},
  'host-group':b=>{state.ssh.hostGroup=b.dataset.id;render();},
  'delete-profile':b=>{if(state.connections.some(c=>c.jump===b.dataset.id)){toast('该主机被其他配置用作跳板机，请先修改引用。');return;}openDialog('删除主机配置？','<p class="note">删除这个连接模板，不终止已建立的会话。分组与收藏也会移除。</p>',button('取消','close-dialog')+button('删除配置','confirm-delete-profile','danger',`data-id="${escapeHtml(b.dataset.id)}"`));},
  'confirm-delete-profile':b=>{state.connections=state.connections.filter(c=>c.name!==b.dataset.id);closeDialog();render();toast('主机配置已删除（模拟）；已有会话继续运行');}
};
function setupWorkspaceInteractions(){
  document.addEventListener('input',e=>{if(e.target.hasAttribute('data-host-filter')){state.ssh.hostQuery=e.target.value;$('#host-results').innerHTML=hostResults();}});
  document.addEventListener('click',e=>{
    const pane=e.target.closest('[data-pane]');if(!pane||e.target.closest('button, input, form'))return;
    if(workspace()?.focus!==pane.dataset.pane){focusPane(pane.dataset.pane);render();}
  });
  document.addEventListener('submit',e=>{
    const form=e.target.closest('[data-pane-form]');if(!form)return;e.preventDefault();focusPane(form.dataset.paneForm);
    const p=focusedPane(),s=current(),sh=s?.shells.find(sh=>sh.id===p.shell);if(!p.input||!sh||sh.exited)return;
    const text=new FormData(form).get('command').trim();if(!text)return;
    state.output[p.shell]=(state.output[p.shell]||'')+`\n<span class="green">❯ ${escapeHtml(text)}</span>\n<span class="dim">[模拟输入，未执行真实命令]</span>`;s.waiting=false;
    if(text==='exit'){sh.exited=true;p.input=false;}render();document.getElementById('command-'+p.id)?.focus();
  });
  let drag=null;
  function setRatio(id,ratio){const node=findSplit(workspace()?.tree,id);if(!node)return;node.ratio=Math.max(20,Math.min(80,Math.round(ratio)));const el=document.querySelector(`[data-split="${id}"]`);if(el){el.children[0].style.flex=node.ratio;el.children[2].style.flex=100-node.ratio;el.children[1].setAttribute('aria-valuenow',node.ratio);}}
  document.addEventListener('pointerdown',e=>{const divider=e.target.closest('[data-divider]');if(!divider)return;e.preventDefault();const node=findSplit(workspace()?.tree,divider.dataset.divider);if(!node)return;drag={id:node.id,axis:node.axis,rect:divider.parentElement.getBoundingClientRect()};divider.setPointerCapture(e.pointerId);});
  document.addEventListener('pointermove',e=>{if(!drag)return;setRatio(drag.id,100*(drag.axis==='row'?(e.clientX-drag.rect.left)/drag.rect.width:(e.clientY-drag.rect.top)/drag.rect.height));});
  document.addEventListener('pointerup',()=>drag=null);document.addEventListener('pointercancel',()=>drag=null);
  document.addEventListener('keydown',e=>{const divider=e.target.closest('[data-divider]');if(!divider||!['ArrowLeft','ArrowRight','ArrowUp','ArrowDown'].includes(e.key))return;e.preventDefault();const node=findSplit(workspace()?.tree,divider.dataset.divider);setRatio(node.id,node.ratio+(['ArrowLeft','ArrowUp'].includes(e.key)?-5:5));});
}
