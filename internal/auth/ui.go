package auth

import (
	"html/template"
	"log/slog"
	"net/http"
)

// LoginPageData holds data for the login page template.
type LoginPageData struct {
	Message     string
	Error       string
	Verified    bool
	RedirectURI string
	State       string
}

// SuccessPageData holds data for the success page template.
type SuccessPageData struct {
	IDToken   string
	ExpiresIn int
	Email     string
	AccountID string
}

// DashboardPageData holds data for the dashboard page template.
type DashboardPageData struct {
	Email         string
	Name          string
	Organizations []Organization
}

// Organization represents an organization for the dashboard.
type Organization struct {
	ID          string
	Name        string
	Description string
	Role        string
}

// loginPageHTML contains the HTML template for the login and registration page.
const loginPageHTML = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>libops - Sign In</title>
    <script src="https://cdn.tailwindcss.com"></script>
    <style>
        .container { max-width: 400px; }
        input:focus { border-color: #3b82f6; box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.5); }
        .hidden { display: none; }
    </style>
</head>
<body class="bg-gray-50 flex items-center justify-center min-h-screen">
    <div class="container p-6 bg-white rounded-xl shadow-2xl space-y-6">
        <h1 class="text-3xl font-bold text-center text-gray-800">libops</h1>

        {{if .Verified}}
        <div class="bg-green-100 border border-green-400 text-green-700 px-4 py-3 rounded relative" role="alert">
            <span class="block sm:inline">Email verified successfully! You can now log in.</span>
        </div>
        {{end}}

        {{if .Error}}
        <div class="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative" role="alert">
            <span class="block sm:inline">{{.Error}}</span>
        </div>
        {{end}}

        {{if .Message}}
        <div class="bg-blue-100 border border-blue-400 text-blue-700 px-4 py-3 rounded relative" role="alert">
            <span class="block sm:inline">{{.Message}}</span>
        </div>
        {{end}}

        <!-- Login Form (default view) -->
        <div id="login-view">
            <!-- Google Sign In -->
            <div class="space-y-4">
                <h2 class="text-xl font-semibold text-gray-700">Sign in with Google</h2>
                <a href="/auth/google{{if .RedirectURI}}?redirect_uri={{.RedirectURI}}{{if .State}}&state={{.State}}{{end}}{{end}}" class="w-full flex items-center justify-center px-4 py-3 border border-transparent text-sm font-medium rounded-lg text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 transition duration-150">
                    <svg class="w-5 h-5 mr-2" fill="currentColor" viewBox="0 0 48 48">
                        <path d="M24 9.5c3.54 0 6.71 1.22 9.21 3.65l6.85-6.85C35.91 3.2 30.7 1 24 1 14.57 1 6.8 6.55 3.1 14.7l7.98 6.19C13.23 13.9 18.67 9.5 24 9.5z" fill="#EA4335"/>
                        <path d="M46.9 24.5c0-1.63-.14-3.19-.43-4.65H24v9.16h13.62c-.6 3.09-2.27 5.67-4.66 7.47l6.52 5.08c3.84-3.56 6.02-8.81 6.02-15.06z" fill="#4285F4"/>
                        <path d="M10.97 27.28c-.28-.84-.44-1.74-.44-2.7 0-.96.16-1.86.44-2.7l-7.98-6.19C3.7 17.45 2 20.84 2 24.5s1.7 7.05 4.99 9.89l7.98-6.19z" fill="#FBBC05"/>
                        <path d="M24 43.5c5.33 0 9.87-1.8 13.16-4.82l-6.52-5.08c-2.39 1.8-5.77 2.91-9.64 2.91-5.33 0-9.87-3.48-11.53-8.49l-7.98 6.19C6.8 41.45 14.57 47 24 47c5.67 0 10.74-2.19 14.55-5.7l-6.85-6.85C30.7 41.7 27.54 43.5 24 43.5z" fill="#34A853"/>
                    </svg>
                    Sign in with Google
                </a>
            </div>

            <div class="relative flex py-5 items-center">
                <div class="flex-grow border-t border-gray-300"></div>
                <span class="flex-shrink mx-4 text-gray-500 text-sm">OR</span>
                <div class="flex-grow border-t border-gray-300"></div>
            </div>

            <!-- Username/Password Login -->
            <div class="space-y-4">
                <h2 class="text-xl font-semibold text-gray-700">Email & Password</h2>
                <form action="/auth/userpass/login" method="POST" class="space-y-4">
                    {{if .RedirectURI}}
                    <input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">
                    {{end}}
                    {{if .State}}
                    <input type="hidden" name="state" value="{{.State}}">
                    {{end}}
                    <div>
                        <label for="login-email" class="block text-sm font-medium text-gray-700">Email</label>
                        <input type="email" id="login-email" name="email" required class="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-lg shadow-sm placeholder-gray-400 focus:outline-none sm:text-sm">
                    </div>
                    <div>
                        <label for="login-password" class="block text-sm font-medium text-gray-700">Password</label>
                        <input type="password" id="login-password" name="password" required class="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-lg shadow-sm placeholder-gray-400 focus:outline-none sm:text-sm">
                    </div>
                    <button type="submit" class="w-full flex justify-center py-2 px-4 border border-transparent rounded-lg shadow-sm text-sm font-medium text-white bg-green-600 hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500 transition duration-150">
                        Log In
                    </button>
                </form>
            </div>

            <!-- Link to Registration -->
            <div class="text-center pt-4 border-t border-gray-200">
                <p class="text-sm text-gray-600">
                    Don't have an account?
                    <a href="#" onclick="showRegister(); return false;" class="font-medium text-blue-600 hover:text-blue-500">
                        Create one
                    </a>
                </p>
            </div>
        </div>

        <!-- Registration Form (hidden by default) -->
        <div id="register-view" class="hidden">
            <!-- Google Sign Up -->
            <div class="space-y-4">
                <h2 class="text-xl font-semibold text-gray-700">Sign up with Google</h2>
                <a href="/auth/google?register=true{{if .RedirectURI}}&redirect_uri={{.RedirectURI}}{{if .State}}&state={{.State}}{{end}}{{end}}" class="w-full flex items-center justify-center px-4 py-3 border border-transparent text-sm font-medium rounded-lg text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 transition duration-150">
                    <svg class="w-5 h-5 mr-2" fill="currentColor" viewBox="0 0 48 48">
                        <path d="M24 9.5c3.54 0 6.71 1.22 9.21 3.65l6.85-6.85C35.91 3.2 30.7 1 24 1 14.57 1 6.8 6.55 3.1 14.7l7.98 6.19C13.23 13.9 18.67 9.5 24 9.5z" fill="#EA4335"/>
                        <path d="M46.9 24.5c0-1.63-.14-3.19-.43-4.65H24v9.16h13.62c-.6 3.09-2.27 5.67-4.66 7.47l6.52 5.08c3.84-3.56 6.02-8.81 6.02-15.06z" fill="#4285F4"/>
                        <path d="M10.97 27.28c-.28-.84-.44-1.74-.44-2.7 0-.96.16-1.86.44-2.7l-7.98-6.19C3.7 17.45 2 20.84 2 24.5s1.7 7.05 4.99 9.89l7.98-6.19z" fill="#FBBC05"/>
                        <path d="M24 43.5c5.33 0 9.87-1.8 13.16-4.82l-6.52-5.08c-2.39 1.8-5.77 2.91-9.64 2.91-5.33 0-9.87-3.48-11.53-8.49l-7.98 6.19C6.8 41.45 14.57 47 24 47c5.67 0 10.74-2.19 14.55-5.7l-6.85-6.85C30.7 41.7 27.54 43.5 24 43.5z" fill="#34A853"/>
                    </svg>
                    Sign up with Google
                </a>
            </div>

            <div class="relative flex py-5 items-center">
                <div class="flex-grow border-t border-gray-300"></div>
                <span class="flex-shrink mx-4 text-gray-500 text-sm">OR</span>
                <div class="flex-grow border-t border-gray-300"></div>
            </div>

            <!-- Username/Password Registration -->
            <div class="space-y-4">
                <h2 class="text-xl font-semibold text-gray-700">Create Account</h2>
                <form action="/auth/userpass/register" method="POST" class="space-y-4">
                    {{if .RedirectURI}}
                    <input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">
                    {{end}}
                    {{if .State}}
                    <input type="hidden" name="state" value="{{.State}}">
                    {{end}}
                    <div>
                        <label for="register-email" class="block text-sm font-medium text-gray-700">Email</label>
                        <input type="email" id="register-email" name="email" required class="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-lg shadow-sm placeholder-gray-400 focus:outline-none sm:text-sm">
                    </div>
                    <div>
                        <label for="register-password" class="block text-sm font-medium text-gray-700">Password</label>
                        <input type="password" id="register-password" name="password" required minlength="8" class="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-lg shadow-sm placeholder-gray-400 focus:outline-none sm:text-sm">
                        <p class="mt-1 text-xs text-gray-500">At least 8 characters with uppercase, lowercase, number, and special character</p>
                    </div>
                    <button type="submit" class="w-full flex justify-center py-2 px-4 border border-transparent rounded-lg shadow-sm text-sm font-medium text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 transition duration-150">
                        Create Account
                    </button>
                </form>
            </div>

            <!-- Link to Login -->
            <div class="text-center pt-4 border-t border-gray-200">
                <p class="text-sm text-gray-600">
                    Already have an account?
                    <a href="#" onclick="showLogin(); return false;" class="font-medium text-blue-600 hover:text-blue-500">
                        Sign in
                    </a>
                </p>
            </div>
        </div>
    </div>

    <script>
        function showRegister() {
            document.getElementById('login-view').classList.add('hidden');
            document.getElementById('register-view').classList.remove('hidden');
        }

        function showLogin() {
            document.getElementById('register-view').classList.add('hidden');
            document.getElementById('login-view').classList.remove('hidden');
        }

        // Show register view if URL has ?register=true
        if (window.location.search.includes('register=true')) {
            showRegister();
        }
    </script>
    <script src="/static/token.js"></script>
</body>
</html>
`

// RenderLoginPage renders the login/registration page.
func RenderLoginPage(w http.ResponseWriter, data LoginPageData) {
	tmpl, err := template.New("login").Parse(loginPageHTML)
	if err != nil {
		slog.Error("Failed to parse login template", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		slog.Error("Failed to render login template", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// HandleLoginPage handles requests to the login page.
func HandleLoginPage(w http.ResponseWriter, r *http.Request) {
	data := LoginPageData{}

	// Check for query parameters
	if r.URL.Query().Get("verified") == "true" {
		data.Verified = true
	}

	if msg := r.URL.Query().Get("message"); msg != "" {
		data.Message = msg
	}

	if err := r.URL.Query().Get("error"); err != "" {
		data.Error = err
	}

	// Extract CLI redirect parameters
	if redirectURI := r.URL.Query().Get("redirect_uri"); redirectURI != "" {
		data.RedirectURI = redirectURI
	}

	if state := r.URL.Query().Get("state"); state != "" {
		data.State = state
	}

	RenderLoginPage(w, data)
}

// successPageHTML contains the HTML template for the success page after authentication.
const successPageHTML = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>libops - Authentication Successful</title>
    <script src="https://cdn.tailwindcss.com"></script>
</head>
<body class="bg-gray-50 flex items-center justify-center min-h-screen">
    <div class="max-w-2xl p-6 bg-white rounded-xl shadow-2xl space-y-6">
        <div class="text-center">
            <svg class="mx-auto h-16 w-16 text-green-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"></path>
            </svg>
            <h1 class="text-3xl font-bold text-gray-800 mt-4">Authentication Successful!</h1>
            <p class="text-gray-600 mt-2">You have been successfully authenticated.</p>
        </div>

        <div class="bg-gray-50 p-4 rounded-lg space-y-2">
            <div class="text-sm text-gray-700">
                <span class="font-semibold">Email:</span> {{.Email}}
            </div>
            <div class="text-sm text-gray-700">
                <span class="font-semibold">Account ID:</span> {{.AccountID}}
            </div>
            <div class="text-sm text-gray-700">
                <span class="font-semibold">Token expires in:</span> {{.ExpiresIn}} seconds
            </div>
        </div>

        <div class="space-y-2">
            <p class="text-sm font-semibold text-gray-700">Your authentication token:</p>
            <div class="bg-gray-100 p-3 rounded border border-gray-300 break-all font-mono text-xs">
                {{.IDToken}}
            </div>
            <button onclick="copyToken()" class="w-full px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition duration-150">
                Copy Token to Clipboard
            </button>
        </div>

        <div class="text-center text-sm text-gray-600">
            <p>You can close this window and return to the CLI.</p>
        </div>
    </div>

    <script>
        function copyToken() {
            const token = "{{.IDToken}}";
            navigator.clipboard.writeText(token).then(() => {
                alert('Token copied to clipboard!');
            }).catch(err => {
                console.error('Failed to copy token:', err);
            });
        }
    </script>
</body>
</html>
`

// RenderSuccessPage renders the authentication success page.
func RenderSuccessPage(w http.ResponseWriter, data SuccessPageData) {
	tmpl, err := template.New("success").Parse(successPageHTML)
	if err != nil {
		slog.Error("Failed to parse success template", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		slog.Error("Failed to render success template", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// dashboardHTML contains the HTML template for the dashboard page.
const dashboardHTML = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>libops - Dashboard</title>
    <link href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.2/dist/css/bootstrap.min.css" rel="stylesheet">
    <style>
        body {
            background-color: #f8f9fa;
        }
        .navbar {
            box-shadow: 0 2px 4px rgba(0,0,0,.1);
        }
        .main-content {
            padding: 2rem 0;
        }
        .card {
            box-shadow: 0 0.125rem 0.25rem rgba(0,0,0,.075);
        }
        .table-responsive {
            border-radius: 0.5rem;
            overflow: hidden;
        }
    </style>
</head>
<body>
    <!-- Navigation -->
    <nav class="navbar navbar-expand-lg navbar-dark bg-primary">
        <div class="container">
            <a class="navbar-brand fw-bold" href="/dashboard">libops</a>
            <button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#navbarNav">
                <span class="navbar-toggler-icon"></span>
            </button>
            <div class="collapse navbar-collapse" id="navbarNav">
                <ul class="navbar-nav ms-auto">
                    <li class="nav-item dropdown">
                        <a class="nav-link dropdown-toggle" href="#" role="button" data-bs-toggle="dropdown">
                            {{if .Name}}{{.Name}}{{else}}{{.Email}}{{end}}
                        </a>
                        <ul class="dropdown-menu dropdown-menu-end">
                            <li><a class="dropdown-item" href="/auth/me">Profile</a></li>
                            <li><hr class="dropdown-divider"></li>
                            <li><a class="dropdown-item" href="/auth/logout">Sign out</a></li>
                        </ul>
                    </li>
                </ul>
            </div>
        </div>
    </nav>

    <!-- Main Content -->
    <div class="container main-content">
        <div class="row mb-4">
            <div class="col">
                <h1 class="h2">Welcome, {{if .Name}}{{.Name}}{{else}}{{.Email}}{{end}}</h1>
                <p class="text-muted">Manage your organizations and projects</p>
            </div>
        </div>

        <div class="row">
            <div class="col">
                <div class="card">
                    <div class="card-header bg-white">
                        <h5 class="card-title mb-0">Your Organizations</h5>
                    </div>
                    <div class="card-body p-0">
                        {{if .Organizations}}
                        <div class="table-responsive">
                            <table class="table table-hover mb-0">
                                <thead class="table-light">
                                    <tr>
                                        <th>Name</th>
                                        <th>Description</th>
                                        <th>Your Role</th>
                                        <th>Actions</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {{range .Organizations}}
                                    <tr>
                                        <td class="fw-semibold">{{.Name}}</td>
                                        <td>{{if .Description}}{{.Description}}{{else}}<span class="text-muted">No description</span>{{end}}</td>
                                        <td>
                                            {{if eq .Role "owner"}}
                                            <span class="badge bg-primary">Owner</span>
                                            {{else if eq .Role "developer"}}
                                            <span class="badge bg-success">Developer</span>
                                            {{else if eq .Role "read"}}
                                            <span class="badge bg-info">Read Only</span>
                                            {{else}}
                                            <span class="badge bg-secondary">{{.Role}}</span>
                                            {{end}}
                                        </td>
                                        <td>
                                            <a href="/organizations/{{.ID}}" class="btn btn-sm btn-outline-primary">View</a>
                                        </td>
                                    </tr>
                                    {{end}}
                                </tbody>
                            </table>
                        </div>
                        {{else}}
                        <div class="text-center py-5">
                            <svg class="text-muted mb-3" width="48" height="48" fill="currentColor" viewBox="0 0 16 16">
                                <path d="M15 14s1 0 1-1-1-4-5-4-5 3-5 4 1 1 1 1h8zm-7.978-1A.261.261 0 0 1 7 12.996c.001-.264.167-1.03.76-1.72C8.312 10.629 9.282 10 11 10c1.717 0 2.687.63 3.24 1.276.593.69.758 1.457.76 1.72l-.008.002a.274.274 0 0 1-.014.002H7.022zM11 7a2 2 0 1 0 0-4 2 2 0 0 0 0 4zm3-2a3 3 0 1 1-6 0 3 3 0 0 1 6 0zM6.936 9.28a5.88 5.88 0 0 0-1.23-.247A7.35 7.35 0 0 0 5 9c-4 0-5 3-5 4 0 .667.333 1 1 1h4.216A2.238 2.238 0 0 1 5 13c0-1.01.377-2.042 1.09-2.904.243-.294.526-.569.846-.816zM4.92 10A5.493 5.493 0 0 0 4 13H1c0-.26.164-1.03.76-1.724.545-.636 1.492-1.256 3.16-1.275zM1.5 5.5a3 3 0 1 1 6 0 3 3 0 0 1-6 0zm3-2a2 2 0 1 0 0 4 2 2 0 0 0 0-4z"/>
                            </svg>
                            <p class="text-muted">You are not a member of any organizations yet.</p>
                            <a href="/organizations/new" class="btn btn-primary">Create Organization</a>
                        </div>
                        {{end}}
                    </div>
                </div>
            </div>
        </div>
    </div>

    <script src="https://cdn.jsdelivr.net/npm/bootstrap@5.3.2/dist/js/bootstrap.bundle.min.js"></script>
    <script src="/static/token.js"></script>
</body>
</html>
`

// RenderDashboardPage renders the dashboard page.
func RenderDashboardPage(w http.ResponseWriter, data DashboardPageData) {
	tmpl, err := template.New("dashboard").Parse(dashboardHTML)
	if err != nil {
		slog.Error("Failed to parse dashboard template", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		slog.Error("Failed to render dashboard template", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}
