document.addEventListener('DOMContentLoaded', function() {
    const loginForm = document.getElementById('loginForm');
    const changePasswordForm = document.getElementById('changePasswordForm');
    const changePasswordLink = document.getElementById('changePasswordLink');
    const cancelChangePassword = document.getElementById('cancelChangePassword');

    // 切换显示登录表单和修改密码表单
    changePasswordLink.addEventListener('click', function(e) {
        e.preventDefault();
        loginForm.style.display = 'none';
        changePasswordForm.style.display = 'block';
    });

    cancelChangePassword.addEventListener('click', function() {
        loginForm.style.display = 'block';
        changePasswordForm.style.display = 'none';
    });

    // 处理密码修改
    changePasswordForm.addEventListener('submit', async function(e) {
        e.preventDefault();
        
        const currentPassword = document.getElementById('currentPassword').value;
        const newPassword = document.getElementById('newPassword').value;
        const confirmPassword = document.getElementById('confirmPassword').value;

        if (newPassword !== confirmPassword) {
            showError('新密码和确认密码不匹配');
            return;
        }

        try {
            const response = await fetch('/login/change_password', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    current_password: currentPassword,
                    new_password: newPassword
                })
            });

            const data = await response.json();
            if (data.success) {
                alert('密码修改成功，请使用新密码登录');
                loginForm.style.display = 'block';
                changePasswordForm.style.display = 'none';
                changePasswordForm.reset();
            } else {
                showError(data.message || '密码修改失败');
            }
        } catch (error) {
            showError('密码修改请求失败: ' + error.message);
        }
    });

    
    loginForm.addEventListener('submit', async function(e) {
        e.preventDefault();
        
        const username = document.getElementById('username').value;
        const password = document.getElementById('password').value;
        
        // 验证用户名和密码
        if (username !== 'admin') {
            showError('用户名不正确');
            return;
        }
        
        try {
            // 发送登录请求
            const response = await fetch('/login/', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    username: username,
                    password: password
                })
            });
            
            const data = await response.json();
            
            if (data.success) {
                // 存储token到localStorage
                localStorage.setItem('authToken', data.token);
                
                // 直接跳转到主页
                window.location.href = '/';
            } else {
                showError(data.message || '登录失败');
            }
        } catch (error) {
            showError('登录请求失败: ' + error.message);
        }
    });
});

function showError(message) {
    // 移除现有的错误消息
    const existingError = document.querySelector('.error-message');
    if (existingError) {
        existingError.remove();
    }
    
    // 创建新的错误消息元素
    const errorElement = document.createElement('div');
    errorElement.className = 'error-message';
    errorElement.textContent = message;
    
    // 添加到表单下方
    const loginForm = document.getElementById('loginForm');
    loginForm.appendChild(errorElement);
}