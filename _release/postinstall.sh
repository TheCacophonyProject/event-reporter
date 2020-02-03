#!/bin/bash

systemctl daemon-reload

systemctl enable event-reporter.service
systemctl restart event-reporter.service

systemctl enable service-watcher.service
systemctl restart service-watcher.service
