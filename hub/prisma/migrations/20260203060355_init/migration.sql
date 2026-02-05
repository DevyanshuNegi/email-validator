-- CreateEnum
CREATE TYPE "EmailStatus" AS ENUM ('PENDING', 'VALID', 'INVALID', 'CATCH_ALL', 'GREYLISTED', 'UNKNOWN');

-- CreateEnum
CREATE TYPE "JobStatus" AS ENUM ('PENDING', 'PROCESSING', 'COMPLETED', 'FAILED');

-- CreateTable
CREATE TABLE "Job" (
    "id" TEXT NOT NULL,
    "totalEmails" INTEGER NOT NULL,
    "status" "JobStatus" NOT NULL DEFAULT 'PENDING',
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "Job_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "EmailCheck" (
    "id" TEXT NOT NULL,
    "jobId" TEXT NOT NULL,
    "email" TEXT NOT NULL,
    "status" "EmailStatus" NOT NULL DEFAULT 'PENDING',
    "smtpCode" INTEGER,
    "bounceReason" TEXT,

    CONSTRAINT "EmailCheck_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE INDEX "EmailCheck_jobId_idx" ON "EmailCheck"("jobId");

-- AddForeignKey
ALTER TABLE "EmailCheck" ADD CONSTRAINT "EmailCheck_jobId_fkey" FOREIGN KEY ("jobId") REFERENCES "Job"("id") ON DELETE CASCADE ON UPDATE CASCADE;
