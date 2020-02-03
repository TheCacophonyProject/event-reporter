#!/bin/bash

systemctl daemon-reload
systemctl enable event-reporter.service
systemctl enable service-watcher.service
