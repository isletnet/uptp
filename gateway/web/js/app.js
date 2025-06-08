// API 基础 URL
const API_BASE_URL = '/resource';
const PROXY_CLIENT_API_BASE_URL = '/proxy_client';

// 模态框实例
let resourceModal;
let proxyClientModal;

// 页面加载完成后初始化
document.addEventListener('DOMContentLoaded', function() {
    resourceModal = new bootstrap.Modal(document.getElementById('resourceModal'));
    gatewayNameModal = new bootstrap.Modal(document.getElementById('gatewayNameModal'));
    proxyClientModal = new bootstrap.Modal(document.getElementById('proxyClientModal'));
    loadResources();
    loadGatewayInfo();
    loadProxyClients();
});

// 加载资源列表
async function loadResources() {
    try {
        const response = await fetch(`${API_BASE_URL}/list`);
        const data = await response.json();
        
        if (data.code === 0) {
            const resources = Array.isArray(data.data) ? data.data : [];
            renderResourceList(resources);
        } else {
            showError('加载资源列表失败：' + data.message);
        }
    } catch (error) {
        showError('加载资源列表失败：' + error.message);
    }
}

// 渲染资源列表
function renderResourceList(resources) {
    const tbody = document.getElementById('resourceList');
    tbody.innerHTML = '';

    if (!resources || resources.length === 0) {
        const tr = document.createElement('tr');
        tr.innerHTML = '<td colspan="6" class="text-center">暂无数据</td>';
        tbody.appendChild(tr);
        return;
    }

    resources.forEach(resource => {
        const tr = document.createElement('tr');
        tr.innerHTML = `
            <td class="d-flex align-items-center gap-1">
                <span>${resource.id}</span>
                <button class="btn btn-sm btn-light" onclick="copyToClipboard('${resource.id}')" title="复制ID">
                    <i class="bi bi-clipboard"></i>
                </button>
            </td>
            <td>${resource.name}</td>
            <td>${resource.network}</td>
            <td>${resource.target_addr}</td>
            <td>${resource.target_port}</td>
            <td>
                <button class="btn btn-sm btn-outline-primary" onclick="editResource('${resource.id.toString()}')">
                    <i class="bi bi-pencil"></i>
                </button>
                <button class="btn btn-sm btn-outline-danger ms-1" onclick="deleteResource('${resource.id.toString()}')">
                    <i class="bi bi-trash"></i>
                </button>
            </td>
        `;
        tbody.appendChild(tr);
    });
}

// 显示添加资源模态框
function showAddModal() {
    document.getElementById('modalTitle').textContent = '添加资源';
    document.getElementById('resourceForm').reset();
    document.getElementById('resourceId').value = '';
    resourceModal.show();
}

// 编辑资源
async function editResource(id) {
    try {
        const response = await fetch(`${API_BASE_URL}/get/${id}`);
        const data = await response.json();
        
        if (data.code === 0) {
            const resource = data.data;
            document.getElementById('modalTitle').textContent = '编辑资源';
            document.getElementById('resourceId').value = resource.id.toString();
            document.getElementById('name').value = resource.name;
            document.getElementById('network').value = resource.network;
            document.getElementById('targetAddr').value = resource.target_addr;
            document.getElementById('targetPort').value = resource.target_port;
            resourceModal.show();
        } else {
            showError('获取资源信息失败：' + data.message);
        }
    } catch (error) {
        showError('获取资源信息失败：' + error.message);
    }
}

// 保存资源
async function saveResource() {
    const form = document.getElementById('resourceForm');
    if (!form.checkValidity()) {
        form.reportValidity();
        return;
    }

    const resourceId = document.getElementById('resourceId').value;
    const resource = {
        name: document.getElementById('name').value,
        network: document.getElementById('network').value,
        target_addr: document.getElementById('targetAddr').value,
        target_port: parseInt(document.getElementById('targetPort').value),
    };

    try {
        const url = resourceId ? `${API_BASE_URL}/update` : `${API_BASE_URL}/add`;
        if (resourceId) {
            resource.id = resourceId;
        }

        const response = await fetch(url, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(resource)
        });

        const data = await response.json();
        if (data.code === 0) {
            resourceModal.hide();
            loadResources();
        } else {
            showError('保存资源失败：' + data.message);
        }
    } catch (error) {
        showError('保存资源失败：' + error.message);
    }
}

// 删除资源
async function deleteResource(id) {
    if (!confirm('确定要删除这个资源吗？')) {
        return;
    }

    try {
        const response = await fetch(`${API_BASE_URL}/delete`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({ id: id })
        });

        const data = await response.json();
        if (data.code === 0) {
            loadResources();
        } else {
            showError('删除资源失败：' + data.message);
        }
    } catch (error) {
        showError('删除资源失败：' + error.message);
    }
}

// 加载网关信息
async function loadGatewayInfo() {
    try {
        const response = await fetch('/gateway/info');
        const data = await response.json();
        
        if (data.code === 0) {
            document.getElementById('gatewayId').textContent = data.data.p2p_id;
            document.getElementById('gatewayName').textContent = data.data.name || '未设置';
            document.getElementById('gatewayPort').textContent = data.data.running_port;
            document.getElementById('gatewayToken').textContent = data.data.token;
            document.getElementById('gatewayVersion').textContent = data.data.version || '未知';
        } else {
            showError('加载网关信息失败：' + data.message);
        }
    } catch (error) {
        showError('加载网关信息失败：' + error.message);
    }
}

// 检查更新
async function upgradeGateway() {
    try {
        const response = await fetch('/upgrade/myself', {
            method: 'GET'
        });
        
        const data = await response.json();
        if (data.code === 0) {
            if (data.message.includes('latest')) {
                alert('当前已是最新版本');
            } else if (data.message.includes('success')) {
                if (confirm('升级成功，是否立即重启网关？')) {
                    fetch('/gateway/restart', {method: 'GET'});
                    alert('正在重启网关，请稍后手动刷新网页查看...');
                }
            } else {
                alert(data.message || '升级操作已完成');
            }
        } else {
            showError(data.message || '升级失败');
        }
    } catch (error) {
        showError('请求失败：' + error.message);
    }
}

// 显示修改网关名称模态框
function showGatewayNameModal() {
    document.getElementById('newGatewayName').value = document.getElementById('gatewayName').textContent;
    gatewayNameModal.show();
}

// 更新网关名称
async function updateGatewayName() {
    const newName = document.getElementById('newGatewayName').value.trim();
    if (!newName) {
        alert('请输入网关名称');
        return;
    }

    try {
        const response = await fetch('/gateway/name', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({ name: newName })
        });

        const data = await response.json();
        if (data.code === 0) {
            document.getElementById('gatewayName').textContent = newName;
            gatewayNameModal.hide();
        } else {
            showError('更新网关名称失败：' + data.message);
        }
    } catch (error) {
        showError('更新网关名称失败：' + error.message);
    }
}

// 通用复制到剪贴板函数
function copyToClipboard(text) {
    if (!text) {
        showError('没有可复制的内容');
        return;
    }

    const textarea = document.createElement('textarea');
    textarea.value = text;
    textarea.style.position = 'fixed';
    document.body.appendChild(textarea);
    textarea.select();

    try {
        const successful = document.execCommand('copy');
        if (successful) {
            const toast = new bootstrap.Toast(document.getElementById('copyToast'));
            toast.show();
        } else {
            showError('复制失败，请手动复制');
        }
    } catch (err) {
        showError('复制失败: ' + err);
    } finally {
        document.body.removeChild(textarea);
    }
}

// 复制网关ID到剪贴板
function copyGatewayId() {
    const gatewayId = document.getElementById('gatewayId').textContent.trim();
    copyToClipboard(gatewayId);
}

// 复制网关Token到剪贴板
function copyGatewayToken() {
    const gatewayToken = document.getElementById('gatewayToken').textContent.trim();
    copyToClipboard(gatewayToken);
}

// 显示错误信息
function showError(message) {
    alert(message);
}

// 下载Android客户端
function downloadAndroidClient() {
    window.location.href = '/upgrade/agent/android';
}

// 加载代理出口列表
async function loadProxyClients() {
    try {
        const response = await fetch(`${PROXY_CLIENT_API_BASE_URL}/list`);
        const data = await response.json();
        
        if (data.code === 0) {
            const clients = Array.isArray(data.data) ? data.data : [];
            renderProxyClients(clients);
        } else {
            showError('加载代理出口列表失败：' + data.message);
        }
    } catch (error) {
        showError('加载代理出口列表失败：' + error.message);
    }
}

// 渲染代理出口列表
function renderProxyClients(clients) {
    const tbody = document.getElementById('proxyClientList');
    tbody.innerHTML = '';

    if (!clients || clients.length === 0) {
        const tr = document.createElement('tr');
        tr.innerHTML = '<td colspan="6" class="text-center">暂无数据</td>';
        tbody.appendChild(tr);
        return;
    }

    clients.forEach(client => {
        const tr = document.createElement('tr');
        tr.innerHTML = `
            <td>${client.remark}</td>
            <td>${client.route}</td>
            <td>${client.peer_name}</td>
            <td>
                <span class="badge bg-${client.open ? 'success' : 'secondary'}">
                    ${client.open ? '启用' : '停用'}
                </span>
            </td>
            <td>
                <button class="btn btn-sm btn-outline-${client.open ? 'danger' : 'success'}" onclick="toggleProxyClientStatus('${client.id}', ${!client.open})">
                    <i class="bi bi-${client.open ? 'pause' : 'play'}"></i>
                </button>
                <button class="btn btn-sm btn-outline-primary ms-1" onclick="editProxyClient('${client.id}')">
                    <i class="bi bi-pencil"></i>
                </button>
                <button class="btn btn-sm btn-outline-danger ms-1" onclick="deleteProxyClient('${client.id}')">
                    <i class="bi bi-trash"></i>
                </button>
            </td>
        `;
        tbody.appendChild(tr);
    });
}

// 显示添加代理出口模态框
function showAddProxyClientModal() {
    document.getElementById('proxyClientModalTitle').textContent = '添加代理出口';
    document.getElementById('proxyClientForm').reset();
    document.getElementById('proxyClientId').value = '';
    proxyClientModal.show();
}

// 编辑代理出口
async function editProxyClient(id) {
    try {
        const response = await fetch(`${PROXY_CLIENT_API_BASE_URL}/get/${id}`);
        const data = await response.json();
        
        if (data.code === 0) {
            const client = data.data;
            document.getElementById('proxyClientModalTitle').textContent = '编辑代理出口';
            document.getElementById('proxyClientId').value = client.id;
            document.getElementById('peerId').value = client.peer;
            document.getElementById('clientToken').value = client.token;
            document.getElementById('remark').value = client.remark || '';
            document.getElementById('route').value = client.route || '0.0.0.0/0';
            document.getElementById('open').checked = client.open !== false;
            proxyClientModal.show();
        } else {
            showError('获取代理出口信息失败：' + data.message);
        }
    } catch (error) {
        showError('获取代理出口信息失败：' + error.message);
    }
}

// 保存代理出口
async function saveProxyClient() {
    const form = document.getElementById('proxyClientForm');
    if (!form.checkValidity()) {
        form.reportValidity();
        return;
    }

    const clientId = document.getElementById('proxyClientId').value;
    const client = {
        peer: document.getElementById('peerId').value,
        token: document.getElementById('clientToken').value,
        remark: document.getElementById('remark').value || '',
        open: document.getElementById('open').checked,
        route: document.getElementById('route').value || '0.0.0.0/0'
    };

    try {
        const url = clientId ? `${PROXY_CLIENT_API_BASE_URL}/update` : `${PROXY_CLIENT_API_BASE_URL}/add`;
        if (clientId) {
            client.id = clientId;
        }

        const response = await fetch(url, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(client)
        });

        const data = await response.json();
        if (data.code === 0) {
            proxyClientModal.hide();
            loadProxyClients();
        } else {
            showError('保存代理出口失败：' + data.message);
        }
    } catch (error) {
        showError('保存代理出口失败：' + error.message);
    }
}

// 切换代理出口状态
async function toggleProxyClientStatus(id, newStatus) {
    try {
        // 先获取当前配置
        const getResponse = await fetch(`${PROXY_CLIENT_API_BASE_URL}/get/${id}`);
        const getData = await getResponse.json();
        
        if (getData.code !== 0) {
            showError('获取代理出口信息失败：' + getData.message);
            return;
        }

        const client = getData.data;
        // 更新状态
        client.open = newStatus;

        // 提交更新
        const updateResponse = await fetch(`${PROXY_CLIENT_API_BASE_URL}/update`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(client)
        });

        const updateData = await updateResponse.json();
        if (updateData.code === 0) {
            loadProxyClients();
        } else {
            showError('切换状态失败：' + updateData.message);
        }
    } catch (error) {
        showError('切换状态失败：' + error.message);
    }
}

// 删除代理出口
async function deleteProxyClient(id) {
    if (!confirm('确定要删除此代理出口吗？')) {
        return;
    }

    try {
        const response = await fetch(`${PROXY_CLIENT_API_BASE_URL}/delete`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({ id: id })
        });
        const data = await response.json();
        
        if (data.code === 0) {
            loadProxyClients();
        } else {
            showError('删除代理出口失败：' + data.message);
        }
    } catch (error) {
        showError('删除代理出口失败：' + error.message);
    }
}