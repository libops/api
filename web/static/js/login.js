function validateEmailForm() {
    const email = document.getElementById('login-email');
    const btn = document.getElementById('email-continue');
    if (email.value && email.validity.valid) {
        btn.disabled = false;
        btn.classList.add('active');
    } else {
        btn.disabled = true;
        btn.classList.remove('active');
    }
}

function showPasswordStep() {
    document.getElementById('email-step').classList.add('hidden');
    document.getElementById('password-step').classList.remove('hidden');
}

function hidePasswordStep() {
    document.getElementById('password-step').classList.add('hidden');
    document.getElementById('email-step').classList.remove('hidden');
}

function showRegister() {
    document.getElementById('login-view').classList.add('hidden');
    document.getElementById('register-view').classList.remove('hidden');
}

function showLogin() {
    document.getElementById('register-view').classList.add('hidden');
    document.getElementById('login-view').classList.remove('hidden');
}

if (window.location.search.includes('register=true')) {
    showRegister();
}
