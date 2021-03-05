adu = load('allocation-decision.latencies')
adm = adu / 1000.0
aeu = load('allocation-enforcement.latencies')
aem = aeu / 1000.0
rdu = load('release-decision.latencies')
rdm = rdu / 1000.0
reu = load('release-enforcement.latencies')
rem = reu / 1000.0

tens = [10 20 30 40 50 60 70 80 90 100]
admp = prctile(adm, tens)
aemp = prctile(aem, tens)
rdmp = prctile(rdm, tens)
remp = prctile(rem, tens)

rows = 2
columns = 4
idx = 1

subplot(rows, columns, idx++)
hist(adm, 100)
title("Allocation Decision")
xlabel("latency / ms")
ylabel("# of allocations")
grid on
subplot(rows, columns, idx++)
plot(admp, tens)
title("Percentile")
ylabel("latency / ms")
grid on

subplot(rows, columns, idx++)
hist(aem, 100)
title("Allocation Enforcement")
xlabel("latency / ms")
ylabel("# of allocations")
grid on
subplot(rows, columns, idx++)
plot(aemp, tens)
title("Percentile")
ylabel("latency / ms")
grid on

subplot(rows, columns, idx++)
hist(rdm, 100)
title("Release Decision")
xlabel("latency / ms")
ylabel("# of releases")
grid on
subplot(rows, columns, idx++)
plot(rdmp, tens)
title("Percentile")
ylabel("latency / ms")
grid on

subplot(rows, columns, idx++)
hist(rem, 100)
title("Release Enforcement")
xlabel("latency / ms")
ylabel("# of releases")
grid on
subplot(rows, columns, idx++)
plot(remp, tens)
title("Percentile")
ylabel("latency / ms")
grid on
