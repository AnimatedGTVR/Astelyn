import 'dart:async';
import 'dart:convert';

import 'package:http/http.dart' as http;

/// Override at build time: `--dart-define=API_URL=https://api.example.com`.
/// Android emulators reach the host machine at http://10.0.2.2:8080.
const apiUrl = String.fromEnvironment(
  'API_URL',
  defaultValue: 'http://localhost:8080',
);

class ApiException implements Exception {
  ApiException(this.message, {this.status});

  final String message;
  final int? status;

  bool get isUnauthorized => status == 401;

  @override
  String toString() => message;
}

class User {
  const User({required this.id, required this.username, required this.email});

  factory User.fromJson(Map<String, dynamic> json) => User(
    id: json['id'] as String,
    username: json['username'] as String,
    email: json['email'] as String,
  );

  final String id;
  final String username;
  final String email;
}

class AuthResult {
  const AuthResult({required this.token, required this.user});

  final String token;
  final User user;
}

class AuthApi {
  AuthApi({http.Client? client, String? baseUrl})
    : _client = client ?? http.Client(),
      _baseUrl = baseUrl ?? apiUrl;

  final http.Client _client;
  final String _baseUrl;

  Future<AuthResult> register({
    required String username,
    required String email,
    required String password,
  }) async {
    final json = await _send(
      'POST',
      '/v1/auth/register',
      body: {'username': username, 'email': email, 'password': password},
    );
    return _authResult(json);
  }

  Future<AuthResult> login({
    required String identifier,
    required String password,
  }) async {
    final json = await _send(
      'POST',
      '/v1/auth/login',
      body: {'identifier': identifier, 'password': password},
    );
    return _authResult(json);
  }

  Future<User> me(String token) async {
    final json = await _send('GET', '/v1/auth/me', token: token);
    return User.fromJson(json);
  }

  Future<void> logout(String token) async {
    await _send('POST', '/v1/auth/logout', token: token);
  }

  AuthResult _authResult(Map<String, dynamic> json) => AuthResult(
    token: json['token'] as String,
    user: User.fromJson(json['user'] as Map<String, dynamic>),
  );

  Future<Map<String, dynamic>> _send(
    String method,
    String path, {
    Map<String, dynamic>? body,
    String? token,
  }) async {
    final request = http.Request(method, Uri.parse('$_baseUrl$path'));
    if (body != null) {
      request.headers['Content-Type'] = 'application/json';
      request.body = jsonEncode(body);
    }
    if (token != null) request.headers['Authorization'] = 'Bearer $token';

    final http.Response response;
    try {
      response = await http.Response.fromStream(
        await _client.send(request).timeout(const Duration(seconds: 15)),
      );
    } on TimeoutException {
      throw ApiException('The server took too long to respond.');
    } catch (_) {
      throw ApiException('Could not reach the server. Check your connection.');
    }

    Map<String, dynamic> json = {};
    if (response.body.isNotEmpty) {
      try {
        json = jsonDecode(response.body) as Map<String, dynamic>;
      } catch (_) {}
    }
    if (response.statusCode >= 400) {
      throw ApiException(
        json['error'] as String? ??
            'Something went wrong (${response.statusCode}).',
        status: response.statusCode,
      );
    }
    return json;
  }
}
