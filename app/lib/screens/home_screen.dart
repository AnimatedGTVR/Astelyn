import 'package:flutter/material.dart';

import '../auth/auth_controller.dart';

class HomeScreen extends StatelessWidget {
  const HomeScreen({super.key, required this.auth});

  final AuthController auth;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('✦ Astelyn'),
        actions: [
          TextButton(onPressed: auth.logout, child: const Text('Log out')),
        ],
      ),
      body: Center(
        child: Text(
          'Welcome, ${auth.user?.username ?? ''}',
          style: Theme.of(context).textTheme.headlineMedium,
        ),
      ),
    );
  }
}
